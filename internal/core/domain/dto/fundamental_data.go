package dto

// FundamentalData es el merge de domain.Fundamentals (dividendos) con lo
// que ya calculamos en domain.IntradaySnapshot (OHLC/volumen del dia,
// derivados de las velas propias) -- son dos fuentes internas separadas,
// pero el frontend espera un solo objeto (FundamentalData en
// screener.models.ts). El resto de sus campos opcionales (marketCap,
// sharesOutstanding, beta, etc.) no tienen fuente todavia -- ver
// project_marketdata_fundamentals_known_limits, quedan ausentes a proposito
// en vez de inventar un valor.
type FundamentalData struct {
	Symbol            string  `json:"symbol"`
	IsEtf             bool    `json:"isEtf"`
	DividendAmount    float64 `json:"dividendAmount"`
	DividendFrequency float64 `json:"dividendFrequency"`
	DayVolume         int64   `json:"dayVolume"`
	Open              float64 `json:"open"`
	High              float64 `json:"high"`
	Low               float64 `json:"low"`
	PrevClose         float64 `json:"prevClose"`
	PreMarketVolume   int64   `json:"preMarketVolume"`
	PostMarketVolume  int64   `json:"postMarketVolume"`
}
