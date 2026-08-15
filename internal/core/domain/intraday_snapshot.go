package domain

import "time"

// IntradaySnapshot es todo lo que se puede armar de fundamentales SOLO con
// nuestras propias velas M1/D1, sin pedirle nada a una fuente externa --
// precio/volumen ya vienen gratis del streaming en vivo que ya mantenemos.
type IntradaySnapshot struct {
	Symbol string
	AsOf   time.Time

	CurrentPrice  float64
	CurrentVolume int64

	Open      float64
	High      float64
	Low       float64
	PrevClose float64
	DayVolume int64

	PreMarketVolume  int64
	PreMarketClose   float64
	PostMarketVolume int64
	PostMarketClose  float64
}
