package domain

// Tags json en el formato que ya espera el frontend (FundamentalData) --
// DividendFrequency queda numerico por ahora aunque el frontend lo tipa
// como string ("Quarterly" etc.); ver nota en get_fundamentals.go.
//
// Campos de /market-data/by-type y /market-metrics de TastyTrade,
// confirmados con respuestas reales antes de agregarlos (ver
// cmd/verify-market-data, cmd/verify-metrics) -- shortInterest/shortRatio/
// sharesOutstanding/openInterest quedan afuera a proposito: no vinieron en
// ninguna respuesta real (o dependen de FINRA/SEC, fuera de alcance).
type Fundamentals struct {
	Symbol            string  `json:"symbol"`
	IsEtf             bool    `json:"isEtf"`
	DividendAmount    float64 `json:"dividendAmount"`
	DividendFrequency float64 `json:"dividendFrequency"`

	TradingStatus string `json:"tradingStatus"`
	HaltStartTime int64  `json:"-"`
	HaltEndTime   int64  `json:"-"`

	MarketCap                   float64 `json:"marketCap"`
	Eps                         float64 `json:"eps"`
	Beta                        float64 `json:"beta"`
	Lendability                 string  `json:"lendability"`
	BorrowRate                  float64 `json:"borrowRate"`
	Liquidity                   float64 `json:"liquidity"`
	LiquidityRating             int     `json:"liquidityRating"`
	ImpliedVolatilityIndex      float64 `json:"impliedVolatilityIndex"`
	ImpliedVolatilityRank       float64 `json:"impliedVolatilityRank"`
	ImpliedVolatilityPercentile float64 `json:"impliedVolatilityPercentile"`
	NextEarningsDate            string  `json:"nextEarningsDate"`
}
