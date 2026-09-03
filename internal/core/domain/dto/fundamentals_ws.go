package dto

import "github.com/MeTradingPlat/marketdata-service/internal/core/domain"

// FundamentalsMessage lleva domain.Fundamentals crudo -- se manda solo
// cuando FundamentalsCache.ReloadAll corre de verdad (barrido nocturno o el
// loop de trading status cada 15 min), nunca por tick. Es baja frecuencia a
// proposito: estos campos no cambian mas seguido que eso (ver
// project_marketdata_fundamentals_known_limits).
type FundamentalsMessage struct {
	Type         string              `json:"type"`
	Symbol       string              `json:"symbol"`
	Fundamentals domain.Fundamentals `json:"fundamentals"`
}

type FundamentalsControlMessage struct {
	Type    string `json:"type"`
	Symbol  string `json:"symbol,omitempty"`
	Message string `json:"message,omitempty"`
}
