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
