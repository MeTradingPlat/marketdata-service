package dto

// FundamentalData es el merge de cuatro fuentes: domain.Fundamentals
// (dividendos + /market-data/by-type + /market-metrics, todo TastyTrade;
// mas sharesOutstanding/floatShares/shortInterest/shortRatio via SEC
// EDGAR+FINRA), domain.IntradaySnapshot (OHLC/volumen del dia, derivado de
// nuestras propias velas) y SecurityType/DaysUntilEarnings (calculados al
// vuelo, no almacenados). openInterest sigue sin fuente (necesitaria datos
// de opciones, fuera de alcance).
//
// Los 4 campos de SEC EDGAR/FINRA son punteros -- a diferencia del resto de
// este DTO, para poder distinguir "todavia no corrio ese refresco" (nil,
// omitido del JSON) de "corrio y no encontro dato" (tambien nil, mismo
// tratamiento: no hay forma de diferenciarlos aca sin tocar el resto del
// DTO, y no hace falta -- el frontend ya trata ambos como "N/A" por igual).
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

	MarketCap                   float64  `json:"marketCap"`
	SharesOutstanding           *int64   `json:"sharesOutstanding,omitempty"`
	FloatShares                 *int64   `json:"floatShares,omitempty"`
	FloatSource                 *string  `json:"floatSource,omitempty"`
	ShortInterest               *float64 `json:"shortInterest,omitempty"`
	ShortRatio                  *float64 `json:"shortRatio,omitempty"`
	Eps                         float64  `json:"eps"`
	Beta                        float64  `json:"beta"`
	Lendability                 string   `json:"lendability"`
	BorrowRate                  float64  `json:"borrowRate"`
	Liquidity                   float64  `json:"liquidity"`
	LiquidityRating             int      `json:"liquidityRating"`
	ImpliedVolatilityIndex      float64  `json:"impliedVolatilityIndex"`
	ImpliedVolatilityRank       float64  `json:"impliedVolatilityRank"`
	ImpliedVolatilityPercentile float64  `json:"impliedVolatilityPercentile"`
	NextEarningsDate            string   `json:"nextEarningsDate"`
	DaysUntilEarnings           int      `json:"daysUntilEarnings"`
	// OccurredDate es la fecha del ultimo reporte de earnings REAL (con EPS
	// ya publicado) -- el frontend la muestra como "Last Report Date".
	// "" (no puntero) es consistente con el resto de campos de texto de
	// este DTO: una fecha ISO nunca es la cadena vacia.
	OccurredDate string `json:"occurredDate"`
}
