package dto

import "github.com/MeTradingPlat/marketdata-service/internal/core/domain"

// SymbolDetails combina Symbol + FundamentalData en la forma que espera
// GET /marketdata/symbols/{symbol}/details -- envelope de respuesta puro.
type SymbolDetails struct {
	ActiveEquity    domain.Symbol   `json:"activeEquity"`
	FundamentalData FundamentalData `json:"fundamentalData"`
}
