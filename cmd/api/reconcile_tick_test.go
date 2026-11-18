package main

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

var errChannelTimeout = errors.New("opening dxlink channel timed out")

func fixedNow(t time.Time) func() time.Time { return func() time.Time { return t } }

func manySymbols(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("S%02d", i)
	}
	return out
}

func TestTickStartsEverySymbolWhenHealthy(t *testing.T) {
	var calls atomic.Int64
	start := func(context.Context, string) error { calls.Add(1); return nil }

	outcome := runReconcileTick(context.Background(), manySymbols(20), start, newReconcileBackoff(), time.Now)

	if outcome.started != 20 || outcome.failed != 0 || outcome.tripped || calls.Load() != 20 {
		t.Fatalf("outcome=%+v calls=%d, want 20 started and nothing else", outcome, calls.Load())
	}
}

func TestTickRecordsIsolatedFailureInBackoff(t *testing.T) {
	base := time.Now()
	backoff := newFixedBackoff()
	start := func(_ context.Context, symbol string) error {
		if symbol == "B" {
			return errChannelTimeout
		}
		return nil
	}

	outcome := runReconcileTick(context.Background(), []string{"A", "B", "C", "D"}, start, backoff, fixedNow(base))

	if outcome.started != 3 || outcome.failed != 1 || outcome.tripped {
		t.Fatalf("outcome=%+v, want 3 started, 1 failed, not tripped", outcome)
	}
	if backoff.due("B", base) || !backoff.due("A", base) {
		t.Fatal("only the failed symbol must be in backoff")
	}
}

func TestTickStopsAfterConsecutiveFailuresAndSkipsBackoff(t *testing.T) {
	base := time.Now()
	backoff := newFixedBackoff()
	var calls atomic.Int64
	start := func(context.Context, string) error { calls.Add(1); return errChannelTimeout }
	symbols := manySymbols(40)

	outcome := runReconcileTick(context.Background(), symbols, start, backoff, fixedNow(base))

	if !outcome.tripped {
		t.Fatal("expected the tick to abort after consecutive failures")
	}
	if got := calls.Load(); got >= int64(len(symbols)) {
		t.Fatalf("attempts = %d, want far fewer than %d", got, len(symbols))
	}
	for _, s := range symbols {
		if !backoff.due(s, base) {
			t.Fatalf("%s must not be penalised when the transport is failing", s)
		}
	}
}

func TestTickSuccessClearsExistingBackoff(t *testing.T) {
	base := time.Now()
	backoff := newFixedBackoff()
	backoff.recordFailure("A", base)
	start := func(context.Context, string) error { return nil }

	runReconcileTick(context.Background(), []string{"A"}, start, backoff, fixedNow(base))

	if !backoff.due("A", base) {
		t.Fatal("a successful start must clear the backoff")
	}
}

func TestTickCancelledByParentContextRecordsNothing(t *testing.T) {
	base := time.Now()
	backoff := newFixedBackoff()
	ctx, cancel := context.WithCancel(context.Background())
	start := func(c context.Context, _ string) error { <-c.Done(); return c.Err() }
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()

	outcome := runReconcileTick(ctx, manySymbols(10), start, backoff, fixedNow(base))

	if outcome.started != 0 || outcome.failed != 0 {
		t.Fatalf("outcome=%+v, want nothing counted after cancellation", outcome)
	}
	for _, s := range manySymbols(10) {
		if !backoff.due(s, base) {
			t.Fatalf("%s must not be penalised by a cancelled tick", s)
		}
	}
}

func TestTickRunsAtMostConfiguredWorkersInParallel(t *testing.T) {
	var running, peak atomic.Int64
	start := func(context.Context, string) error {
		now := running.Add(1)
		for {
			old := peak.Load()
			if now <= old || peak.CompareAndSwap(old, now) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		running.Add(-1)
		return nil
	}

	runReconcileTick(context.Background(), manySymbols(30), start, newReconcileBackoff(), time.Now)

	if peak.Load() > reconcileWorkers {
		t.Fatalf("peak parallelism = %d, want <= %d", peak.Load(), reconcileWorkers)
	}
}
