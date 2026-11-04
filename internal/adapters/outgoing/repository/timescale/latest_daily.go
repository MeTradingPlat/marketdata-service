package timescale

import (
	"sort"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func latestDailyBySymbol(candles []domain.Candle) []domain.Candle {
	latest := make(map[string]domain.Candle)
	for _, c := range candles {
		if c.Timeframe != domain.D1 {
			continue
		}
		curr, ok := latest[c.Symbol]
		if !ok || c.Timestamp.After(curr.Timestamp) {
			latest[c.Symbol] = c
		}
	}
	result := make([]domain.Candle, 0, len(latest))
	for _, c := range latest {
		result = append(result, c)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Symbol < result[j].Symbol
	})
	return result
}
