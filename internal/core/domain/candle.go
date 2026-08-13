package domain

import "time"

type Candle struct {
	Symbol     string
	Timeframe  Timeframe
	Timestamp  time.Time
	Open       float64
	High       float64
	Low        float64
	Close      float64
	Volume     int64
	TradeCount int
	VWAP       float64
	Source     string
}

func (c Candle) IsComplete() bool {
	return !c.Timestamp.IsZero() && c.Open != 0 && c.High != 0 && c.Low != 0 && c.Close != 0
}
