package catchup

import (
	"context"
	"sync"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
)

type openInterestBatchResult struct {
	found         map[string]float64
	attemptedOnly []string
	errCount      int
	lastErr       error
}

func fetchOpenInterestBatch(ctx context.Context, gateway out.OpenInterestGateway, stale []string) openInterestBatchResult {
	jobs := make(chan string, len(stale))
	for _, s := range stale {
		jobs <- s
	}
	close(jobs)

	var (
		mu  sync.Mutex
		res = openInterestBatchResult{found: make(map[string]float64)}
		wg  sync.WaitGroup
	)
	for i := 0; i < openInterestWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runOpenInterestWorker(ctx, gateway, jobs, &mu, &res)
		}()
	}
	wg.Wait()
	return res
}

func runOpenInterestWorker(ctx context.Context, gateway out.OpenInterestGateway, jobs <-chan string, mu *sync.Mutex, res *openInterestBatchResult) {
	for symbol := range jobs {
		if ctx.Err() != nil {
			return
		}
		val, ok, err := gateway.OpenInterest(ctx, symbol)
		mu.Lock()
		recordOpenInterestResult(res, symbol, val, ok, err)
		mu.Unlock()
	}
}

func recordOpenInterestResult(res *openInterestBatchResult, symbol string, val float64, ok bool, err error) {
	if err != nil {
		res.errCount++
		res.lastErr = err
	} else if ok && val > 0 {
		res.found[symbol] = val
	} else {
		res.attemptedOnly = append(res.attemptedOnly, symbol)
	}
}
