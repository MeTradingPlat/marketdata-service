package tastytrade

import (
	"context"
	"errors"
	"testing"
	"time"
)

var fastDelays = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}

func TestRetryWithBackoffStopsOnFirstSuccess(t *testing.T) {
	calls := 0
	err := retryWithBackoff(context.Background(), fastDelays, func() error {
		calls++
		return nil
	})
	if err != nil || calls != 1 {
		t.Fatalf("err=%v calls=%d, want nil and 1", err, calls)
	}
}

func TestRetryWithBackoffSucceedsAfterFailures(t *testing.T) {
	calls := 0
	err := retryWithBackoff(context.Background(), fastDelays, func() error {
		calls++
		if calls < 3 {
			return errors.New("timeout")
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Fatalf("err=%v calls=%d, want nil and 3", err, calls)
	}
}

func TestRetryWithBackoffReturnsLastErrorWhenExhausted(t *testing.T) {
	calls := 0
	err := retryWithBackoff(context.Background(), fastDelays, func() error {
		calls++
		return errors.New("timeout")
	})
	if err == nil || calls != len(fastDelays)+1 {
		t.Fatalf("err=%v calls=%d, want error and %d", err, calls, len(fastDelays)+1)
	}
}

func TestRetryWithBackoffStopsWhenContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := retryWithBackoff(ctx, []time.Duration{time.Hour}, func() error {
		calls++
		cancel()
		return errors.New("timeout")
	})
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("err=%v calls=%d, want context.Canceled and 1", err, calls)
	}
}
