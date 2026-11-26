package catchup

import (
	"context"
	"sync"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
)

func runUncoveredPhase(ctx context.Context, ingest in.IngestCandlesService, uncovered []job, workers int) []string {
	var (
		emptyMu sync.Mutex
		empty   []string
		wg      sync.WaitGroup
	)
	queue := make(chan job, len(uncovered))
	for _, j := range uncovered {
		queue <- j
	}
	close(queue)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range queue {
				if backfillWithRetry(ctx, ingest, j) {
					emptyMu.Lock()
					empty = append(empty, j.symbol)
					emptyMu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	return empty
}

func mergeBatchResult(mu *sync.Mutex, empty *[]string, partialBatches, skippedLive *int, batchEmpty []string, partial bool, skipped int) {
	if len(batchEmpty) == 0 && !partial && skipped == 0 {
		return
	}
	mu.Lock()
	if len(batchEmpty) > 0 {
		*empty = append(*empty, batchEmpty...)
	}
	if partial {
		*partialBatches++
	}
	*skippedLive += skipped
	mu.Unlock()
}
