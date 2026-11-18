package tastytrade

import (
	"context"
	"errors"
	"testing"
	"time"
)

var errAllocate = errors.New("allocating channel: context deadline exceeded")

func TestResubscribeSucceedsFirstTryWithoutSpendingBudget(t *testing.T) {
	calls, budget := 0, 5
	err := resubscribeWithRetry(context.Background(), func() error { calls++; return nil }, &budget, time.Millisecond)

	if err != nil || calls != 1 || budget != 5 {
		t.Fatalf("err=%v calls=%d budget=%d, want nil, 1, 5", err, calls, budget)
	}
}

func TestResubscribeRecoversOnRetryWithoutSpendingBudget(t *testing.T) {
	calls, budget := 0, 5
	subscribe := func() error {
		calls++
		if calls == 1 {
			return errAllocate
		}
		return nil
	}

	err := resubscribeWithRetry(context.Background(), subscribe, &budget, time.Millisecond)

	if err != nil || calls != 2 || budget != 5 {
		t.Fatalf("err=%v calls=%d budget=%d, want nil, 2, 5", err, calls, budget)
	}
}

func TestResubscribeSpendsBudgetWhenRetryAlsoFails(t *testing.T) {
	calls, budget := 0, 5
	err := resubscribeWithRetry(context.Background(), func() error { calls++; return errAllocate }, &budget, time.Millisecond)

	if err == nil || calls != 2 || budget != 4 {
		t.Fatalf("err=%v calls=%d budget=%d, want error, 2, 4", err, calls, budget)
	}
}

func TestResubscribeDoesNotRetryOnceBudgetIsSpent(t *testing.T) {
	calls, budget := 0, 0
	err := resubscribeWithRetry(context.Background(), func() error { calls++; return errAllocate }, &budget, time.Millisecond)

	if err == nil || calls != 1 {
		t.Fatalf("err=%v calls=%d, want error after a single call", err, calls)
	}
}

func TestResubscribeStopsWaitingWhenContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls, budget := 0, 5
	subscribe := func() error { calls++; cancel(); return errAllocate }

	err := resubscribeWithRetry(ctx, subscribe, &budget, time.Hour)

	if !errors.Is(err, errAllocate) || calls != 1 {
		t.Fatalf("err=%v calls=%d, want the first error after one call", err, calls)
	}
}
