package domain

import "time"

// Tags json en el formato que ya espera el frontend (FundamentalData) --
// DividendFrequency queda numerico por ahora aunque el frontend lo tipa
// como string ("Quarterly" etc.); ver nota en get_fundamentals.go.
//
// MetricsUpdatedAt/MarketDataUpdatedAt/ExternalUpdatedAt son nil cuando ese
// refresco en particular todavia nunca corrio para el simbolo -- distinto
// de "corrio y el valor real es 0" (ej. market cap de un ETF, dividendo de
// una SPAC). Sin esto no hay forma de diferenciar "no sabemos" de "sabemos
// que es cero" al armar /marketdata/fundamentals/realtime.
type Fundamentals struct {
	Symbol            string  `json:"symbol"`
	IsEtf             bool    `json:"isEtf"`
	DividendAmount    float64 `json:"dividendAmount"`
	DividendFrequency float64 `json:"dividendFrequency"`

	TradingStatus       string     `json:"tradingStatus"`
	StatusReason        string     `json:"-"`
	HaltStartTime       int64      `json:"-"`
	HaltEndTime         int64      `json:"-"`
	MarketDataUpdatedAt *time.Time `json:"-"`

	MarketCap                   float64    `json:"marketCap"`
	Eps                         float64    `json:"eps"`
	Beta                        float64    `json:"beta"`
	Lendability                 string     `json:"lendability"`
	BorrowRate                  float64    `json:"borrowRate"`
	Liquidity                   float64    `json:"liquidity"`
	LiquidityRating             int        `json:"liquidityRating"`
	ImpliedVolatilityIndex      float64    `json:"impliedVolatilityIndex"`
	ImpliedVolatilityRank       float64    `json:"impliedVolatilityRank"`
	ImpliedVolatilityPercentile float64    `json:"impliedVolatilityPercentile"`
	NextEarningsDate            string     `json:"nextEarningsDate"`
	MetricsUpdatedAt            *time.Time `json:"-"`

	// Campos de FINRA/SEC EDGAR -- fuera del alcance de TastyTrade por
	// completo (confirmado contra /market-metrics, /market-data/by-type e
	// /instruments/equities reales, ninguna los trae). nil hasta que
	// RefreshExternalFundamentals corra por primera vez para el simbolo.
	SharesOutstanding *int64   `json:"-"`
	FloatShares       *int64   `json:"-"`
	ShortInterest     *float64 `json:"-"`
	ShortRatio        *float64 `json:"-"`
	// ShortInterestShares es el dato crudo de FINRA (acciones en corto en
	// la fecha de settlement) -- el porcentaje se calcula al leer contra
	// el float vigente (ver ShortInterestPercent), no al guardar.
	ShortInterestShares     *int64     `json:"-"`
	ShortInterestSettlement string     `json:"-"`
	ExternalUpdatedAt       *time.Time `json:"-"`

	// InsiderShares/InsiderCiks son intermedios para calcular floatShares
	// real (ver RefreshBeneficialOwners) -- nunca se exponen en
	// /marketdata/fundamentals/realtime, signal-processing-service no los
	// pide. FloatUpdatedAt solo se toca cuando floatShares pasa de
	// heuristico (90% de sharesOutstanding) a real (SEC EDGAR), para poder
	// rotar por "el que hace mas tiempo no se refresca de verdad".
	InsiderShares  *int64     `json:"-"`
	InsiderCiks    []string   `json:"-"`
	FloatUpdatedAt *time.Time `json:"-"`

	// OccurredDate es la fecha del ultimo reporte de earnings REAL (con EPS
	// ya publicado, no estimado) -- endpoint separado de TastyTrade
	// (historic-corporate-events/earnings-reports), por-simbolo. "" (no
	// puntero) sigue el mismo criterio que NextEarningsDate: una fecha ISO
	// nunca es la cadena vacia, asi que "" ya distingue sin ambiguedad
	// "todavia no se busco" de un dato real.
	OccurredDate      string     `json:"-"`
	EarningsUpdatedAt *time.Time `json:"-"`
}
