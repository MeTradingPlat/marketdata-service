package tastytrade

import (
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

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

// subscribeTrade/unsubscribeTrade: mismo patron de lote entero en un solo
// mensaje que subscribeProfile/unsubscribeProfile -- pedido puntual de
// snapshot, no una suscripcion viva por simbolo.
func (c *dxLinkChannel) subscribeTrade(symbols []string) error {
	items := make([]feedSubscriptionItem, len(symbols))
	for i, s := range symbols {
		items[i] = feedSubscriptionItem{Symbol: s, Type: "Trade"}
	}
	return c.client.send(feedSubscriptionMessage{Type: "FEED_SUBSCRIPTION", Channel: c.id, Add: items})
}

func (c *dxLinkChannel) unsubscribeTrade(symbols []string) error {
	items := make([]feedSubscriptionItem, len(symbols))
	for i, s := range symbols {
		items[i] = feedSubscriptionItem{Symbol: s, Type: "Trade"}
	}
	return c.client.send(feedSubscriptionMessage{Type: "FEED_SUBSCRIPTION", Channel: c.id, Remove: items})
}

// subscribeLiveDayVolume agrega Trade+TradeETH de un simbolo al MISMO canal
// que ya tiene su Candle en vivo (mismo mensaje FEED_SUBSCRIPTION, sin abrir
// canal ni conexion nueva) -- se queda viva para siempre igual que
// subscribeLive, no es un snapshot puntual como subscribeTrade.
func (c *dxLinkChannel) subscribeLiveDayVolume(symbol string) error {
	return c.client.send(feedSubscriptionMessage{
		Type: "FEED_SUBSCRIPTION", Channel: c.id,
		Add: []feedSubscriptionItem{
			{Symbol: symbol, Type: "Trade"},
			{Symbol: symbol, Type: "TradeETH"},
		},
	})
}

func (c *dxLinkChannel) unsubscribeLiveDayVolume(symbol string) error {
	return c.client.send(feedSubscriptionMessage{
		Type: "FEED_SUBSCRIPTION", Channel: c.id,
		Remove: []feedSubscriptionItem{
			{Symbol: symbol, Type: "Trade"},
			{Symbol: symbol, Type: "TradeETH"},
		},
	})
}
