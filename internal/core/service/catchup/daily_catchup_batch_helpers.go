package catchup

import (
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/ingestion"
)

func filterBatchNonLive(gateway out.MarketDataGateway, batch []job, tf domain.Timeframe) ([]string, int) {
	symbols := make([]string, 0, len(batch))
	skippedLive := 0
	for _, j := range batch {
		if tf == domain.M1 && gateway.LiveSubscribed(j.symbol) {
			skippedLive++
			continue
		}
		symbols = append(symbols, j.symbol)
	}
	return symbols, skippedLive
}

func buildBatchFroms(symbols []string, watermarks map[string]time.Time, tf domain.Timeframe, duration time.Duration) map[string]time.Time {
	froms := make(map[string]time.Time, len(symbols))
	for _, sym := range symbols {
		newest, ok := watermarks[sym]
		if ok {
			if ingestion.HasNewClosedBar(newest, duration) {
				froms[sym] = newest.Add(-ingestion.IncrementalMargin * duration)
			}
			continue
		}
		if tf == domain.M1 {
			froms[sym] = time.Now().Add(-noWatermarkM1Fallback)
		}
	}
	return froms
}

func emptyBatchSymbols(froms map[string]time.Time, result map[string][]domain.Candle) []string {
	var empty []string
	for symbol := range froms {
		if len(result[symbol]) == 0 {
			empty = append(empty, symbol)
		}
	}
	return empty
}
