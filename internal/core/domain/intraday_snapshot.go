package domain

import "time"

// IntradaySnapshot es todo lo que se puede armar de fundamentales SOLO con
// nuestras propias velas M1/D1, sin pedirle nada a una fuente externa --
// precio/volumen ya vienen gratis del streaming en vivo que ya mantenemos.
// Tags json en el mismo formato camelCase que el resto del contrato del
// frontend (FundamentalData comparte estos mismos nombres de campo).
type IntradaySnapshot struct {
	Symbol string    `json:"symbol"`
	AsOf   time.Time `json:"asOf"`

	CurrentPrice  float64 `json:"currentPrice"`
	CurrentVolume int64   `json:"currentVolume"`

	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	PrevClose float64 `json:"prevClose"`
	DayVolume int64   `json:"dayVolume"`

	PreMarketVolume  int64   `json:"preMarketVolume"`
	PreMarketClose   float64 `json:"preMarketClose"`
	PostMarketVolume int64   `json:"postMarketVolume"`
	PostMarketClose  float64 `json:"postMarketClose"`
}
