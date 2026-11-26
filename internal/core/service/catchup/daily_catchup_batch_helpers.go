package catchup

import (
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func emptyBatchSymbols(froms map[string]time.Time, result map[string][]domain.Candle) []string {
	var empty []string
	for symbol := range froms {
		if len(result[symbol]) == 0 {
			empty = append(empty, symbol)
		}
	}
	return empty
}
