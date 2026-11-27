package tastytrade

import "github.com/MeTradingPlat/marketdata-service/internal/core/domain"

type historyBatchStats struct {
	requested        int
	withData         int
	signaled         int
	signaledWithData int
	snipped          int
	withoutData      int
}

func computeHistoryBatchStats(requested []string, candles map[string][]domain.Candle, signaled, snipped map[string]bool) historyBatchStats {
	stats := historyBatchStats{requested: len(requested)}
	for _, sym := range requested {
		hasData := len(candles[sym]) > 0
		if hasData {
			stats.withData++
		} else {
			stats.withoutData++
		}
		if signaled[sym] {
			stats.signaled++
			if hasData {
				stats.signaledWithData++
			}
		}
		if snipped[sym] {
			stats.snipped++
		}
	}
	return stats
}
