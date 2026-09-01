package tastytrade

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
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
	id     int
	client *DxLinkConn

	readyOnce sync.Once
	ready     chan struct{}

	mu        sync.RWMutex
	onCandle  func(rawCandleEvent)
	onProfile func(rawProfileEvent)
}

func newDxLinkChannel(id int, client *DxLinkConn) *dxLinkChannel {
	return &dxLinkChannel{id: id, client: client, ready: make(chan struct{})}
}

func (c *dxLinkChannel) open(ctx context.Context) error {
	if err := c.client.send(channelRequestMessage{
		Type: "CHANNEL_REQUEST", Channel: c.id, Service: "FEED", Parameters: map[string]string{"contract": "AUTO"},
	}); err != nil {
		return err
	}
	select {
	case <-c.ready:
		return nil
	// client.Done() cubre el caso donde el socket muere (zombie, sessions
	// exceeded, INVALID_MESSAGE) despues de mandar el CHANNEL_REQUEST pero
	// antes de que CHANNEL_OPENED/FEED_CONFIG lleguen -- sin esto, el unico
	// recurso era ctx, y ctx aca es el del barrido/backfill (vive horas, sin
	// timeout propio), asi que esta espera podia quedar colgada para
	// siempre. Ver el comentario de connDone en dxlink_conn.go.
	case <-c.client.Done():
		return fmt.Errorf("dxlink connection closed while opening channel %d", c.id)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *dxLinkChannel) handleOpened() error {
	return c.client.send(feedSetupMessage{
		Type: "FEED_SETUP", Channel: c.id, AcceptAggregationPeriod: 0.1, AcceptDataFormat: "COMPACT",
		AcceptEventFields: map[string][]string{"Candle": candleEventFields, "Profile": profileEventFields},
	})
}

func (c *dxLinkChannel) handleConfigured() {
	c.readyOnce.Do(func() { close(c.ready) })
}

func (c *dxLinkChannel) handleData(data []interface{}) {
	i := 0
	for i < len(data) {
		typeName, ok := data[i].(string)
		if !ok {
			i++
			continue
		}
		if i+1 >= len(data) {
			i++
			continue
		}
		switch typeName {
		case "Candle":
			if batch, ok := data[i+1].([]interface{}); ok {
				for _, ev := range parseCandleBatch(batch) {
					c.dispatchCandle(ev)
				}
			}
		case "Profile":
			if batch, ok := data[i+1].([]interface{}); ok {
				for _, ev := range parseProfileBatch(batch) {
					c.dispatchProfile(ev)
				}
			}
		}
		i += 2
	}
}

func (c *dxLinkChannel) setOnCandle(fn func(rawCandleEvent)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onCandle = fn
}

func (c *dxLinkChannel) dispatchCandle(ev rawCandleEvent) {
	c.mu.RLock()
	fn := c.onCandle
	c.mu.RUnlock()
	if fn != nil {
		fn(ev)
	}
}

func (c *dxLinkChannel) setOnProfile(fn func(rawProfileEvent)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onProfile = fn
}

func (c *dxLinkChannel) dispatchProfile(ev rawProfileEvent) {
	c.mu.RLock()
	fn := c.onProfile
	c.mu.RUnlock()
	if fn != nil {
		fn(ev)
	}
}

// parseWireSymbol es el inverso de candleSymbol -- separa el simbolo
// desnudo y la temporalidad de lo que llega en un evento crudo. El
// dispatcher central del pool lo usa para saber a cual suscripcion (en
// vivo o historial) pertenece cada evento, ya que un canal pooled puede
// estar sirviendo varias a la vez.
//
// dxFeed omite el "1" de periodo en el simbolo que devuelve para velas de
// periodo 1 -- nos suscribimos con "AAPL{=1d}" pero el FEED_DATA que llega
// trae "AAPL{=d}" (confirmado en vivo: con solo el formato "{=1d}" el
// dispatcher nunca encontraba destinatario y todo se perdia en silencio
// hasta el timeout). Hay que aceptar ambas formas.
func parseWireSymbol(wire string) (symbol string, tf domain.Timeframe, ok bool) {
	idx := strings.Index(wire, "{=")
	if idx < 0 {
		return "", "", false
	}
	switch wire[idx:] {
	case "{=1m}", "{=m}":
		return wire[:idx], domain.M1, true
	case "{=1h}", "{=h}":
		return wire[:idx], domain.H1, true
	case "{=1d}", "{=d}":
		return wire[:idx], domain.D1, true
	default:
		return "", "", false
	}
}

func dxLinkCandleSuffix(tf domain.Timeframe) (string, error) {
	switch tf {
	case domain.M1:
		return "{=1m}", nil
	case domain.H1:
		return "{=1h}", nil
	case domain.D1:
		return "{=1d}", nil
	default:
		return "", fmt.Errorf("unsupported timeframe for dxlink: %s", tf)
	}
}

func candleSymbol(symbol string, tf domain.Timeframe) (string, error) {
	suffix, err := dxLinkCandleSuffix(tf)
	if err != nil {
		return "", err
	}
	return symbol + suffix, nil
}

// dxLinkCandleSuffixNormalized es la forma sin el "1" que dxFeed usa al
// devolver eventos de candles de periodo 1 (ver parseWireSymbol).
func dxLinkCandleSuffixNormalized(tf domain.Timeframe) (string, error) {
	switch tf {
	case domain.M1:
		return "{=m}", nil
	case domain.H1:
		return "{=h}", nil
	case domain.D1:
		return "{=d}", nil
	default:
		return "", fmt.Errorf("unsupported timeframe for dxlink: %s", tf)
	}
}

func candleSymbolNormalized(symbol string, tf domain.Timeframe) (string, error) {
	suffix, err := dxLinkCandleSuffixNormalized(tf)
	if err != nil {
		return "", err
	}
	return symbol + suffix, nil
}

// subscribeLive es la UNICA suscripcion M1 que se manda por simbolo: con
// FromTime distinto de cero, dxLink repite primero el historial desde ese
// punto y despues sigue solo con datos en vivo por la misma suscripcion --
// nunca hay un remove seguido de un add para la misma clave (visto en vivo:
// esa secuencia dejaba el streaming mudo para siempre sin ningun error,
// probablemente el servidor colapsando el remove+add casi simultaneos de la
// misma clave en un solo diff neto).
func (c *dxLinkChannel) subscribeLive(symbol string, tf domain.Timeframe, from time.Time) error {
	sym, err := candleSymbol(symbol, tf)
	if err != nil {
		return err
	}
	item := feedSubscriptionItem{Symbol: sym, Type: "Candle"}
	if !from.IsZero() {
		fromMs := from.UnixMilli()
		item.FromTime = &fromMs
	}
	return c.client.send(feedSubscriptionMessage{
		Type: "FEED_SUBSCRIPTION", Channel: c.id,
		Add: []feedSubscriptionItem{item},
	})
}

func (c *dxLinkChannel) subscribeHistory(symbol string, tf domain.Timeframe, from time.Time) error {
	sym, err := candleSymbol(symbol, tf)
	if err != nil {
		return err
	}
	fromMs := from.UnixMilli()
	return c.client.send(feedSubscriptionMessage{
		Type: "FEED_SUBSCRIPTION", Channel: c.id,
		Add: []feedSubscriptionItem{{Symbol: sym, Type: "Candle", FromTime: &fromMs}},
	})
}

// subscribeHistoryBatch manda el historial de MUCHOS simbolos en un solo
// mensaje FEED_SUBSCRIPTION (cada item con su propio FromTime) -- es el
// agrupamiento original del pool de Java: el barrido pide lotes enteros
// en vez de una suscripcion por simbolo (ver CandlePool.FetchHistoryBatch).
func (c *dxLinkChannel) subscribeHistoryBatch(symbols []string, tf domain.Timeframe, froms map[string]time.Time) error {
	items := make([]feedSubscriptionItem, 0, len(symbols))
	for _, symbol := range symbols {
		sym, err := candleSymbol(symbol, tf)
		if err != nil {
			return err
		}
		fromMs := froms[symbol].UnixMilli()
		items = append(items, feedSubscriptionItem{Symbol: sym, Type: "Candle", FromTime: &fromMs})
	}
	return c.client.send(feedSubscriptionMessage{Type: "FEED_SUBSCRIPTION", Channel: c.id, Add: items})
}

// unsubscribeHistoryBatch manda la baja del lote entero en un solo
// mensaje, con las dos formas del simbolo por item (ver unsubscribe).
func (c *dxLinkChannel) unsubscribeHistoryBatch(symbols []string, tf domain.Timeframe) error {
	items := make([]feedSubscriptionItem, 0, len(symbols)*2)
	for _, symbol := range symbols {
		sym, err := candleSymbol(symbol, tf)
		if err != nil {
			return err
		}
		normalizedSym, err := candleSymbolNormalized(symbol, tf)
		if err != nil {
			return err
		}
		items = append(items,
			feedSubscriptionItem{Symbol: sym, Type: "Candle"},
			feedSubscriptionItem{Symbol: normalizedSym, Type: "Candle"},
		)
	}
	return c.client.send(feedSubscriptionMessage{Type: "FEED_SUBSCRIPTION", Channel: c.id, Remove: items})
}

// unsubscribe manda la baja en las dos formas posibles del simbolo --
// "AAPL{=1d}" (la que usamos para suscribirnos) y "AAPL{=d}" (la
// normalizada que el servidor devuelve en los eventos). No sabemos con
// certeza cual usa dxFeed para encontrar la suscripcion que hay que borrar
// internamente, y una baja que no encuentra coincidencia simplemente no
// hace nada -- sin las dos formas, la suscripcion se queda viva para
// siempre. Confirmado en vivo: con una sola conexion bajo uso sostenido,
// las peticiones se iban volviendo mas lentas con el tiempo (patron
// clasico de fugas de suscripciones acumulandose), y una conexion nueva
// volvia a ser rapida de inmediato.
func (c *dxLinkChannel) unsubscribe(symbol string, tf domain.Timeframe) error {
	sym, err := candleSymbol(symbol, tf)
	if err != nil {
		return err
	}
	normalizedSym, err := candleSymbolNormalized(symbol, tf)
	if err != nil {
		return err
	}
	return c.client.send(feedSubscriptionMessage{
		Type: "FEED_SUBSCRIPTION", Channel: c.id,
		Remove: []feedSubscriptionItem{
			{Symbol: sym, Type: "Candle"},
			{Symbol: normalizedSym, Type: "Candle"},
		},
	})
}

func (c *dxLinkChannel) close() error {
	return c.client.send(channelCancelMessage{Type: "CHANNEL_CANCEL", Channel: c.id})
}

// subscribeProfile manda TODOS los simbolos del lote en un solo mensaje
// FEED_SUBSCRIPTION -- a diferencia de las velas (una suscripcion viva por
// simbolo), Profile es un pedido puntual de snapshot para un lote entero,
// asi que no hace falta un mensaje por simbolo.
func (c *dxLinkChannel) subscribeProfile(symbols []string) error {
	items := make([]feedSubscriptionItem, len(symbols))
	for i, s := range symbols {
		items[i] = feedSubscriptionItem{Symbol: s, Type: "Profile"}
	}
	return c.client.send(feedSubscriptionMessage{Type: "FEED_SUBSCRIPTION", Channel: c.id, Add: items})
}

func (c *dxLinkChannel) unsubscribeProfile(symbols []string) error {
	items := make([]feedSubscriptionItem, len(symbols))
	for i, s := range symbols {
		items[i] = feedSubscriptionItem{Symbol: s, Type: "Profile"}
	}
	return c.client.send(feedSubscriptionMessage{Type: "FEED_SUBSCRIPTION", Channel: c.id, Remove: items})
}
