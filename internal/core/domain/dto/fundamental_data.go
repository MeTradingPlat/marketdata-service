package dto

// FundamentalData es el merge de tres fuentes: domain.Fundamentals
// (dividendos + /market-data/by-type + /market-metrics, todo TastyTrade),
// domain.IntradaySnapshot (OHLC/volumen del dia, derivado de nuestras
// propias velas) y SecurityType/DaysUntilEarnings (calculados al vuelo, no
// almacenados). marketCap/sharesOutstanding-etc. que siguen ausentes
// (shortInterest, shortRatio, openInterest) no tienen fuente sin
// FINRA/SEC/opciones -- ver project_marketdata_fundamentals_known_limits.
type FundamentalData struct {
	Symbol            string  `json:"symbol"`
	IsEtf             bool    `json:"isEtf"`
	SecurityType      string  `json:"securityType"`
	DividendAmount    float64 `json:"dividendAmount"`
	DividendFrequency float64 `json:"dividendFrequency"`
	DayVolume         int64   `json:"dayVolume"`
	Open              float64 `json:"open"`
	High              float64 `json:"high"`
	Low               float64 `json:"low"`
	PrevClose         float64 `json:"prevClose"`
	PreMarketVolume   int64   `json:"preMarketVolume"`
	PostMarketVolume  int64   `json:"postMarketVolume"`

	TradingStatus string `json:"tradingStatus"`

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
	DaysUntilEarnings           int     `json:"daysUntilEarnings"`
}
