package domain

// Tags json en el formato que ya espera el frontend (FundamentalData) --
// DividendFrequency queda numerico por ahora aunque el frontend lo tipa
// como string ("Quarterly" etc.); ver nota en get_fundamentals.go.
type Fundamentals struct {
	Symbol            string  `json:"symbol"`
	IsEtf             bool    `json:"isEtf"`
	DividendAmount    float64 `json:"dividendAmount"`
	DividendFrequency float64 `json:"dividendFrequency"`
}
