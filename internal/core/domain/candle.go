package domain

import "time"

// Tags json en el formato exacto que ya esperaba el frontend del servicio
// Java (HistoricalCandleDTO: symbol/timestamp/open/high/low/close/volume/vwap).
type Candle struct {
	Symbol     string    `json:"symbol"`
	Timeframe  Timeframe `json:"timeframe"`
	Timestamp  time.Time `json:"timestamp"`
	Open       float64   `json:"open"`
	High       float64   `json:"high"`
	Low        float64   `json:"low"`
	Close      float64   `json:"close"`
	Volume     int64     `json:"volume"`
	TradeCount int       `json:"tradeCount"`
	VWAP       float64   `json:"vwap"`
	Source     string    `json:"source"`
}

func (c Candle) IsComplete() bool {
	return !c.Timestamp.IsZero() && c.Open != 0 && c.High != 0 && c.Low != 0 && c.Close != 0
}
