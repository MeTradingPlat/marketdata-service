package main

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

const (
	reconcileWorkers      = 3
	reconcileFailureLimit = 3
	reconcileTickBudget   = 4 * time.Minute
)

type reconcileOutcome struct {
	started int
	failed  int
	tripped bool
}

type startAttempt func(ctx context.Context, symbol string) error

func runReconcileTick(ctx context.Context, symbols []string, start startAttempt, backoff *reconcileBackoff, now func() time.Time) reconcileOutcome {
	tickCtx, cancel := context.WithTimeout(ctx, reconcileTickBudget)
	defer cancel()

	var consecutiveFailures atomic.Int64
	var tripped atomic.Bool
	var mu sync.Mutex
	var started int
	var failed []string

	jobs := make(chan string)
	var wg sync.WaitGroup
	for i := 0; i < reconcileWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for symbol := range jobs {
				if tickCtx.Err() != nil {
					continue
				}
				err := start(tickCtx, symbol)
				if err != nil && tickCtx.Err() != nil {
					continue
				}
				if err == nil {
					consecutiveFailures.Store(0)
					backoff.recordSuccess(symbol)
					mu.Lock()
					started++
					mu.Unlock()
					continue
				}
				mu.Lock()
				failed = append(failed, symbol)
				mu.Unlock()
				if consecutiveFailures.Add(1) >= reconcileFailureLimit {
					tripped.Store(true)
					cancel()
				}
			}
		}()
	}

feed:
	for _, symbol := range symbols {
		select {
		case jobs <- symbol:
		case <-tickCtx.Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()

	if !tripped.Load() {
		for _, symbol := range failed {
			backoff.recordFailure(symbol, now())
		}
	}
	return reconcileOutcome{started: started, failed: len(failed), tripped: tripped.Load()}
}
