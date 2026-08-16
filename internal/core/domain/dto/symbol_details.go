package dto

import "github.com/MeTradingPlat/marketdata-service/internal/core/domain"

// SymbolDetails combina Symbol + Fundamentals en la forma que espera
// GET /marketdata/symbols/{symbol}/details -- envelope de respuesta puro,
// ninguno de los dos tipos que envuelve cambia.
type SymbolDetails struct {
	ActiveEquity    domain.Symbol       `json:"activeEquity"`
	FundamentalData domain.Fundamentals `json:"fundamentalData"`
}
