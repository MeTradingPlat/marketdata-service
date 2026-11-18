package tastytrade

import (
	"context"
	"time"
)

func retryWithBackoff(ctx context.Context, delays []time.Duration, attempt func() error) error {
	err := attempt()
	for _, delay := range delays {
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		err = attempt()
	}
	return err
}
