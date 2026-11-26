package catchup

import (
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/ingestion"
)

func filterBatchNonLiveSymbols(gateway out.MarketDataGateway, batch []job, tf domain.Timeframe) ([]string, []string) {
	symbols := make([]string, 0, len(batch))
	var skipped []string
	for _, j := range batch {
		if tf == domain.M1 && gateway.LiveSubscribed(j.symbol) {
			skipped = append(skipped, j.symbol)
			continue
		}
		symbols = append(symbols, j.symbol)
	}
	return symbols, skipped
}

func partitionBatchWatermarks(symbols []string, watermarks map[string]time.Time, tf domain.Timeframe, duration time.Duration) (map[string]time.Time, []string, []string) {
	froms := make(map[string]time.Time, len(symbols))
	var upToDate, noWatermark []string
	for _, sym := range symbols {
		newest, ok := watermarks[sym]
		if ok {
			if ingestion.HasNewClosedBar(newest, duration) {
				froms[sym] = newest.Add(-ingestion.IncrementalMargin * duration)
			} else {
				upToDate = append(upToDate, sym)
			}
			continue
		}
		if tf == domain.M1 {
			froms[sym] = time.Now().Add(-noWatermarkM1Fallback)
		} else {
			noWatermark = append(noWatermark, sym)
		}
	}
	return froms, upToDate, noWatermark
}

func collectClosedBatch(result map[string][]domain.Candle) ([]domain.Candle, map[string]struct{}, []string) {
	merged := make([]domain.Candle, 0, len(result))
	bySymbol := make(map[string]struct{}, len(result))
	var emptyClosed []string
	for symbol, candlesFetched := range result {
		closed := domain.ClosedCandles(candlesFetched, time.Now())
		if len(closed) == 0 {
			if len(candlesFetched) > 0 {
				emptyClosed = append(emptyClosed, symbol)
			}
			continue
		}
		merged = append(merged, closed...)
		bySymbol[symbol] = struct{}{}
	}
	return merged, bySymbol, emptyClosed
}
