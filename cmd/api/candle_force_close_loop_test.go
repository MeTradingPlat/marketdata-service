package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type fakeCandleForceCloser struct {
	calls atomic.Int64
}

func (f *fakeCandleForceCloser) CloseElapsedForming(now time.Time) {
	f.calls.Add(1)
}

func TestStartCandleForceCloseLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	closer := &fakeCandleForceCloser{}
	StartCandleForceCloseLoop(ctx, closer)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && closer.calls.Load() < 1 {
		time.Sleep(50 * time.Millisecond)
	}
	if closer.calls.Load() < 1 {
		t.Fatalf("closer not invoked within 3s")
	}

	cancel()
	time.Sleep(1500 * time.Millisecond)
	before := closer.calls.Load()
	time.Sleep(1500 * time.Millisecond)
	after := closer.calls.Load()
	if after != before {
		t.Fatalf("loop kept running after cancel: %d -> %d", before, after)
	}
}
