package tastytrade

import (
	"context"
	"time"
)

const (
	resubscribeRetryDelay  = 2 * time.Second
	resubscribeRetryBudget = 5
)

func resubscribeWithRetry(ctx context.Context, subscribe func() error, retriesLeft *int, delay time.Duration) error {
	err := subscribe()
	if err == nil || *retriesLeft <= 0 {
		return err
	}
	select {
	case <-ctx.Done():
		return err
	case <-time.After(delay):
	}
	if err = subscribe(); err != nil {
		*retriesLeft--
	}
	return err
}
