package main

import (
	"context"
	"slices"
	"testing"
	"time"
)

type fakeShutdownFlusher struct {
	calls    []string
	deadline bool
}

func (f *fakeShutdownFlusher) FlushLiveSaves(ctx context.Context) bool {
	_, f.deadline = ctx.Deadline()
	f.calls = append(f.calls, "flush")
	return false
}

func (f *fakeShutdownFlusher) RetryPendingSaves(context.Context) bool {
	f.calls = append(f.calls, "retry")
	return true
}

func (f *fakeShutdownFlusher) PendingSaves() int { return 0 }

func TestFlushSavesOnShutdown_FlushesThenRetriesWithDeadline(t *testing.T) {
	f := &fakeShutdownFlusher{}

	flushSavesOnShutdown(f, time.Second)

	if !slices.Equal(f.calls, []string{"flush", "retry"}) || !f.deadline {
		t.Fatalf("calls = %v, deadline = %v", f.calls, f.deadline)
	}
}
