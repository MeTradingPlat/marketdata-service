package tastytrade

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

var (
	channelOpenTimeout    = 45 * time.Second
	errChannelOpenTimeout = errors.New("dxlink channel open timed out")
)

// eventFlags al final (indice 9, ver candle_event.go): pedido nuevo,
// defensivo -- dxFeed documenta Candle como IndexedEvent con esta propiedad
// (SNAPSHOT_END/SNAPSHOT_SNIP/TX_PENDING, ver kb.dxfeed.com IndexedEvent),
// que le diria a historyCollector con certeza cuando una rafaga historica
// termino de verdad en vez de adivinar por un timeout de reloj (ver
// historyDeepWait en candle_pool.go). Confirmado en vivo el 2026-08-31:
// FXI/PFE/IBIT -- todos simbolos liquidos -- quedaron con profundidad M1
// mucho mas corta que el piso general (~43 dias) del resto del universo,
// cada uno cortado en una fecha distinta, la firma de un timeout que
// corta a mitad de una rafaga todavia activa, no un limite real del
// proveedor. TastyTrade no siempre puebla todos los campos documentados de
// dxFeed (ver el comentario de profileEventFields, mismo problema con
// Profile) -- si eventFlags tampoco llega poblado aca, el codigo cae de
// vuelta al timeout de siempre sin romper nada; se sabra por el log.
var candleEventFields = []string{"eventSymbol", "time", "open", "high", "low", "close", "volume", "VWAP", "impVolatility", "eventFlags"}

// profileEventFields: el feed retail de TastyTrade solo entrega estos 5
// campos del evento Profile de forma confiable -- confirmado empiricamente
// contra 60,000+ eventos reales (freeFloat/beta/tradingStatus/statusReason/
// haltStartTime/haltEndTime nunca llegan poblados via DxLink pese a estar
// documentados en el schema de dxFeed). Pedir solo lo que de verdad llega
// mantiene el payload liviano para lotes de miles de simbolos.
var profileEventFields = []string{"eventSymbol", "shares"}

type dxLinkChannel struct {
	id             int
	client         *DxLinkConn
	retired        atomic.Bool
	openedReceived atomic.Bool

	readyOnce sync.Once
	ready     chan struct{}

	mu         sync.RWMutex
	onCandle   func(rawCandleEvent)
	onProfile  func(rawProfileEvent)
	onTrade    func(rawTradeEvent)
	onTradeETH func(rawTradeETHEvent)
}

func newDxLinkChannel(id int, client *DxLinkConn) *dxLinkChannel {
	return &dxLinkChannel{id: id, client: client, ready: make(chan struct{})}
}

func (c *dxLinkChannel) open(ctx context.Context) error {
	start := time.Now()
	if err := c.client.send(channelRequestMessage{
		Type: "CHANNEL_REQUEST", Channel: c.id, Service: "FEED", Parameters: map[string]string{"contract": "AUTO"},
	}); err != nil {
		return err
	}
	select {
	case <-c.ready:
		c.client.stats.opensSucceeded.Add(1)
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			log.Warn().Int("channel", c.id).Dur("elapsed", elapsed).Msg("opening dxlink channel slow")
		}
		return nil
	// client.Done() cubre el caso donde el socket muere (zombie, sessions
	// exceeded, INVALID_MESSAGE) despues de mandar el CHANNEL_REQUEST pero
	// antes de que CHANNEL_OPENED/FEED_CONFIG lleguen -- sin esto, el unico
	// recurso era ctx, y ctx aca es el del barrido/backfill (vive horas, sin
	// timeout propio), asi que esta espera podia quedar colgada para
	// siempre. Ver el comentario de connDone en dxlink_conn.go.
	case <-c.client.Done():
		return fmt.Errorf("dxlink connection closed while opening channel %d", c.id)
	case <-time.After(channelOpenTimeout):
		return c.recordOpenTimeout(start, time.Since(start))
	case <-ctx.Done():
		c.client.stats.opensCanceled.Add(1)
		return ctx.Err()
	}
}

func (c *dxLinkChannel) recordOpenTimeout(sentAt time.Time, elapsed time.Duration) error {
	timeouts := c.client.stats.opensTimedOut.Add(1)
	canceled := c.client.stats.opensCanceled.Load()
	openChannels := c.client.openChannelCount()
	lastMsgAge := c.client.lastMessageAge()
	cause := classifyOpenTimeout(
		c.client.lastMessageAt(),
		c.client.stats.lastErrorAt(),
		sentAt,
		c.openedReceived.Load(),
		c.client.stats.lateResponses.Load(),
	)
	c.client.stats.setLastTimeoutCause(cause)
	log.Warn().
		Int64("connID", c.client.id).
		Int("channel", c.id).
		Int("open_channels", openChannels).
		Dur("elapsed", elapsed).
		Dur("last_message_age", lastMsgAge).
		Int64("canceled", canceled).
		Int64("timeouts", timeouts).
		Str("probable_cause", cause).
		Msg("dxlink channel open failed")
	return fmt.Errorf("%w: channel %d connID %d open_channels %d elapsed %s last_message_age %s canceled %d timeouts %d cause %s",
		errChannelOpenTimeout, c.id, c.client.id, openChannels, elapsed, lastMsgAge, canceled, timeouts, cause)
}

func (c *dxLinkChannel) handleOpened() error {
	c.openedReceived.Store(true)
	return c.client.send(feedSetupMessage{
		Type: "FEED_SETUP", Channel: c.id, AcceptAggregationPeriod: 0.1, AcceptDataFormat: "COMPACT",
		AcceptEventFields: map[string][]string{
			"Candle": candleEventFields, "Profile": profileEventFields,
			"Trade": tradeEventFields, "TradeETH": tradeETHEventFields,
		},
	})
}

func (c *dxLinkChannel) handleConfigured() {
	c.readyOnce.Do(func() { close(c.ready) })
}
