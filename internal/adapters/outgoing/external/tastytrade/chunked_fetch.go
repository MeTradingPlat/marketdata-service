package tastytrade

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

type chunkPolicy struct {
	size     int
	throttle time.Duration
	attempts int
	backoff  time.Duration
}

type chunkFetcher[T any] func(ctx context.Context, symbols []string) ([]T, error)

func fetchInChunks[T any](ctx context.Context, symbols []string, policy chunkPolicy, fetch chunkFetcher[T]) ([]T, error) {
	var all []T
	var lastErr error
	failed, total := 0, 0
	for i := 0; i < len(symbols); i += policy.size {
		if i > 0 && !sleepOrDone(ctx, policy.throttle) {
			return all, ctx.Err()
		}
		total++
		chunk, err := fetchWithRetry(ctx, symbols[i:min(i+policy.size, len(symbols))], policy, fetch)
		if ctx.Err() != nil {
			return all, ctx.Err()
		}
		if err != nil {
			failed++
			lastErr = err
			continue
		}
		all = append(all, chunk...)
	}
	if failed > 0 {
		log.Warn().Err(lastErr).Int("failedChunks", failed).Int("totalChunks", total).Msg("tastytrade chunked fetch had failing chunks, keeping the rest")
	}
	if failed == total && total > 0 {
		return nil, lastErr
	}
	return all, nil
}

func fetchWithRetry[T any](ctx context.Context, symbols []string, policy chunkPolicy, fetch chunkFetcher[T]) ([]T, error) {
	var lastErr error
	pause := policy.backoff
	for attempt := 1; attempt <= policy.attempts; attempt++ {
		chunk, err := fetch(ctx, symbols)
		if err == nil {
			return chunk, nil
		}
		lastErr = err
		if attempt == policy.attempts || !sleepOrDone(ctx, pause) {
			break
		}
		pause *= 2
	}
	return nil, lastErr
}

func sleepOrDone(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
