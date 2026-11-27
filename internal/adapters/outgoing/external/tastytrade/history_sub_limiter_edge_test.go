package tastytrade

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestHistorySubLimiter_OversizedPassesWhenEmpty(t *testing.T) {
	lim := newHistorySubLimiterWithClock(5, 60*time.Second, time.Now)
	res, err := lim.acquire(context.Background(), 3)
	if err != nil {
		t.Fatalf("oversized acquire on empty window failed: %v", err)
	}
	res.release()
}

func TestHistorySubLimiter_CancelledContextSendsNothing(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.allocator = newChannelAllocator(factory, p.wireChannel, p.handleConnectionReconnect, 1)
	p.historyLimiter = newHistorySubLimiterWithClock(3, 60*time.Second, time.Now)
	res, _ := p.historyLimiter.acquire(context.Background(), 1)
	defer res.release()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.FetchHistoryBatch(ctx, domain.D1, map[string]time.Time{"AAPL": time.Now().Add(-time.Hour)}); !errors.Is(err, context.Canceled) {
		t.Fatalf("FetchHistoryBatch err = %v, want context.Canceled", err)
	}
	if _, err := p.FetchHistory(ctx, "AAPL", domain.D1, time.Now().Add(-time.Hour)); !errors.Is(err, context.Canceled) {
		t.Fatalf("FetchHistory err = %v, want context.Canceled", err)
	}
}
