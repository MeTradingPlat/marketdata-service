package tastytrade

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

var candleEventFields = []string{"eventSymbol", "time", "open", "high", "low", "close", "volume", "VWAP", "impVolatility"}

type dxLinkChannel struct {
	id     int
	client *DxLinkConn

	readyOnce sync.Once
	ready     chan struct{}

	mu       sync.RWMutex
	onCandle func(rawCandleEvent)
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
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *dxLinkChannel) handleOpened() error {
	return c.client.send(feedSetupMessage{
		Type: "FEED_SETUP", Channel: c.id, AcceptAggregationPeriod: 0.1, AcceptDataFormat: "COMPACT",
		AcceptEventFields: map[string][]string{"Candle": candleEventFields},
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
		if typeName == "Candle" {
			if batch, ok := data[i+1].([]interface{}); ok {
				for _, ev := range parseCandleBatch(batch) {
					c.dispatchCandle(ev)
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

func (c *dxLinkChannel) subscribeLive(symbol string, tf domain.Timeframe) error {
	sym, err := candleSymbol(symbol, tf)
	if err != nil {
		return err
	}
	return c.client.send(feedSubscriptionMessage{
		Type: "FEED_SUBSCRIPTION", Channel: c.id,
		Add: []feedSubscriptionItem{{Symbol: sym, Type: "Candle"}},
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
