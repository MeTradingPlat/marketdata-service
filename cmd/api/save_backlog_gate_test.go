package main

import (
	"context"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
)

type fakeBacklogIngest struct {
	in.IngestCandlesService
	pending int
}

func (f *fakeBacklogIngest) PendingSaves() int { return f.pending }

func TestWaitForSaveBacklogWithTimeout(t *testing.T) {
	below := &fakeBacklogIngest{pending: liveRolloutBacklogLimit}
	if timedOut := waitForSaveBacklogWithTimeout(context.Background(), below, 50*time.Millisecond, 5*time.Millisecond); timedOut {
		t.Fatal("expected no timeout when pending is within limit")
	}

	above := &fakeBacklogIngest{pending: liveRolloutBacklogLimit + 1}
	if timedOut := waitForSaveBacklogWithTimeout(context.Background(), above, 25*time.Millisecond, 5*time.Millisecond); !timedOut {
		t.Fatal("expected timeout when pending stays above limit")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if timedOut := waitForSaveBacklogWithTimeout(ctx, above, time.Second, 5*time.Millisecond); timedOut {
		t.Fatal("expected false when context is canceled")
	}
}
