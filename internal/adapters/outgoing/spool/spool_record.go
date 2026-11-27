package spool

import (
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

type spoolCandle struct {
	domain.Candle
	ObservedAt time.Time `json:"observedAt,omitempty"`
}

func toSpoolCandles(candles []domain.Candle) []spoolCandle {
	records := make([]spoolCandle, len(candles))
	for i, c := range candles {
		records[i] = spoolCandle{Candle: c, ObservedAt: c.ObservedAt}
	}
	return records
}

func fromSpoolCandles(records []spoolCandle) []domain.Candle {
	candles := make([]domain.Candle, len(records))
	for i, r := range records {
		c := r.Candle
		c.ObservedAt = r.ObservedAt
		candles[i] = c
	}
	return candles
}
