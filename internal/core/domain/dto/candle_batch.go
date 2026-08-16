package dto

import "github.com/MeTradingPlat/marketdata-service/internal/core/domain"

// CandlesBatchResponse es el envelope que ya espera signal-processing-service
// (fetch_candles en marketdata_client.py) -- domain.Candle ya tiene los tags
// json correctos para cada barra individual (mismo formato que /historical),
// esto solo lo agrupa por simbolo.
type CandlesBatchResponse struct {
	CandlesPorSimbolo map[string][]domain.Candle `json:"candlesPorSimbolo"`
}
