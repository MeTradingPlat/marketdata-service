package dto

// FundamentalRealtime es el contrato exacto que ya espera
// signal-processing-service (FundamentalResponse en marketdata_models.py)
// para POST /marketdata/fundamentals/realtime. Todo puntero a proposito:
// un campo en nil se serializa ausente (omitempty), que Pydantic interpreta
// como None -- si usaramos valores planos, un dato que nunca se pudo
// buscar (nil) seria indistinguible de un dato real en cero (ej. market
// cap de un ETF, dividendo de una SPAC), y el codigo Python SI distingue
// ambos casos (fundamentals_ready() chequea "is not None" explicitamente).
type FundamentalRealtime struct {
	Symbol string `json:"symbol"`

	MarketCap         *float64 `json:"marketCap,omitempty"`
	SharesOutstanding *int64   `json:"sharesOutstanding,omitempty"`
	FloatShares       *int64   `json:"floatShares,omitempty"`
	ShortInterest     *float64 `json:"shortInterest,omitempty"`
	ShortRatio        *float64 `json:"shortRatio,omitempty"`

	DayVolume         *int64   `json:"dayVolume,omitempty"`
	DaysUntilEarnings *int     `json:"daysUntilEarnings,omitempty"`
	PreMarketVolume   *int64   `json:"preMarketVolume,omitempty"`
	PostMarketVolume  *int64   `json:"postMarketVolume,omitempty"`
	// PrevPostMarketVolume: postmarket de la sesion ANTERIOR -- ver
	// domain.Fundamentals.PrevPostMarketVolume sobre por que un filtro
	// TIPO_VOLUMEN=AMBOS corrido en premarket lo necesita (PostMarketVolume
	// de HOY es genuinamente 0 hasta que esa sesion pase).
	PrevPostMarketVolume *int64 `json:"prevPostMarketVolume,omitempty"`
	PreMarketClose    *float64 `json:"preMarketClose,omitempty"`
	PostMarketClose   *float64 `json:"postMarketClose,omitempty"`
	Open              *float64 `json:"open,omitempty"`
	High              *float64 `json:"high,omitempty"`
	Low               *float64 `json:"low,omitempty"`
	PrevClose         *float64 `json:"prevClose,omitempty"`

	Eps               *float64 `json:"eps,omitempty"`
	DividendAmount    *float64 `json:"dividendAmount,omitempty"`
	DividendFrequency *string  `json:"dividendFrequency,omitempty"`

	TradingStatus *string `json:"tradingStatus,omitempty"`
	StatusReason  *string `json:"statusReason,omitempty"`
	HaltStartTime *int64  `json:"haltStartTime,omitempty"`
	HaltEndTime   *int64  `json:"haltEndTime,omitempty"`

	Beta                        *float64 `json:"beta,omitempty"`
	ImpliedVolatilityIndex      *float64 `json:"impliedVolatilityIndex,omitempty"`
	ImpliedVolatilityRank       *float64 `json:"impliedVolatilityRank,omitempty"`
	ImpliedVolatilityPercentile *float64 `json:"impliedVolatilityPercentile,omitempty"`
	Liquidity                   *float64 `json:"liquidity,omitempty"`
	LiquidityRating             *int     `json:"liquidityRating,omitempty"`

	IsEtf        *bool   `json:"isEtf,omitempty"`
	SecurityType *string `json:"securityType,omitempty"`
}
