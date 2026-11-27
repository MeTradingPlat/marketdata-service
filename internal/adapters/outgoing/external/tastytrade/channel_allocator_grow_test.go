package tastytrade

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestChannelAllocator_AllocateRespectsContextWhileGrowBusy(t *testing.T) {
	a := newChannelAllocator(nil, nil, nil, 2)
	if err := a.lockGrow(context.Background()); err != nil {
		t.Fatalf("initial lockGrow: %v", err)
	}
	defer a.unlockGrow()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := a.allocate(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Fatalf("allocate waited too long: %v", elapsed)
	}
}

func TestChannelAllocator_LockGrowCanceledDoesNotLeakSemaphore(t *testing.T) {
	a := newChannelAllocator(nil, nil, nil, 2)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.lockGrow(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected Canceled, got %v", err)
	}
	okCtx, okCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer okCancel()
	if err := a.lockGrow(okCtx); err != nil {
		t.Fatalf("semaphore leaked after canceled lockGrow: %v", err)
	}
	a.unlockGrow()
}
