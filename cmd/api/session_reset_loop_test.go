package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNextResetTime(t *testing.T) {
	now := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)

	got := nextResetTime(now, 22, 15)
	want := time.Date(2026, 11, 14, 22, 15, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("before hour: got %v, want %v", got, want)
	}

	got = nextResetTime(now, 9, 0)
	want = time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("passed hour: got %v, want %v", got, want)
	}
}

type countingResetter struct{ calls int }

func (r *countingResetter) ResetSessions(context.Context) error {
	r.calls++
	return errors.New("boom")
}

func TestRunScheduledReset_OnlyRotatesTheSession(t *testing.T) {
	r := &countingResetter{}

	runScheduledReset(context.Background(), r)

	if r.calls != 1 {
		t.Fatalf("calls = %d, want 1", r.calls)
	}
}
