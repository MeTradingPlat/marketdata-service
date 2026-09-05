package tastytrade

import (
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func mergeCandle(existing domain.Candle, ev rawCandleEvent, symbol string, tf domain.Timeframe) domain.Candle {
	c := existing
	c.Symbol = symbol
	c.Timeframe = tf
	c.Timestamp = ev.Timestamp
	c.Source = "tastytrade"

	if c.Open == 0 && ev.Open != nil {
		c.Open = *ev.Open
	}
	if ev.High != nil && (c.High == 0 || *ev.High > c.High) {
		c.High = *ev.High
	}
	if ev.Low != nil && (c.Low == 0 || *ev.Low < c.Low) {
		c.Low = *ev.Low
	}
	if ev.Close != nil {
		c.Close = *ev.Close
	}
	if ev.Volume != nil {
		c.Volume = int64(*ev.Volume)
	}
	if ev.VWAP != nil {
		c.VWAP = *ev.VWAP
	}
	return c
}

// candleValuesEqual compara solo los campos que mergeCandle puede cambiar --
// usado para no re-despachar (y por lo tanto no re-guardar) una vela ya
// cerrada cuando el refresco periodico de suscripcion (RefreshLiveSubscriptions)
// reproduce el mismo historial reciente sin que nada haya cambiado de
// verdad. time.Time se compara con Equal, no ==, porque dos timestamps del
// mismo instante pueden venir de parseos distintos con Location/monotonic
// distintos.
func candleValuesEqual(a, b domain.Candle) bool {
	return a.Timestamp.Equal(b.Timestamp) &&
		a.Open == b.Open && a.High == b.High && a.Low == b.Low && a.Close == b.Close &&
		a.Volume == b.Volume && a.VWAP == b.VWAP
}
