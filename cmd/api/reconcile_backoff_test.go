package main

import (
	"testing"
	"time"
)

func newFixedBackoff() *reconcileBackoff {
	b := newReconcileBackoff()
	b.jitter = func() float64 { return 0.5 }
	return b
}

func TestBackoffDelayDoublesUntilCap(t *testing.T) {
	b := newFixedBackoff()
	want := []time.Duration{5 * time.Minute, 10 * time.Minute, 20 * time.Minute, 40 * time.Minute, time.Hour, time.Hour, time.Hour}

	for i, expected := range want {
		if got := b.delay(i + 1); got != expected {
			t.Fatalf("delay(%d) = %s, want %s", i+1, got, expected)
		}
	}
}

func TestBackoffJitterStaysWithinBounds(t *testing.T) {
	b := newReconcileBackoff()
	for i := 0; i < 200; i++ {
		got := b.delay(1)
		low := time.Duration(float64(reconcileBackoffBase) * (1 - reconcileBackoffJitter))
		high := time.Duration(float64(reconcileBackoffBase) * (1 + reconcileBackoffJitter))
		if got < low || got > high {
			t.Fatalf("delay(1) = %s, want within [%s, %s]", got, low, high)
		}
	}
}

func TestBackoffBlocksSymbolUntilDelayPasses(t *testing.T) {
	b := newFixedBackoff()
	now := time.Now()

	b.recordFailure("ABC", now)

	if b.due("ABC", now.Add(4*time.Minute)) {
		t.Fatal("symbol must not be due before the delay passes")
	}
	if !b.due("ABC", now.Add(5*time.Minute)) {
		t.Fatal("symbol must be due once the delay passes")
	}
	if !b.due("OTHER", now) {
		t.Fatal("untouched symbols are always due")
	}
}

func TestBackoffSuccessClearsFailures(t *testing.T) {
	b := newFixedBackoff()
	now := time.Now()
	b.recordFailure("ABC", now)

	b.recordSuccess("ABC")

	if !b.due("ABC", now) {
		t.Fatal("symbol must be due right after a success")
	}
	b.recordFailure("ABC", now)
	if got := b.entries["ABC"].failures; got != 1 {
		t.Fatalf("failures = %d, want 1 after reset", got)
	}
}
