package tastytrade

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestFetchHistoryBatch_CancelledAfterSubscribeReleasesChannelAndLocks(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.allocator = newChannelAllocator(factory, p.wireChannel, p.handleConnectionReconnect, 1)
	ch, err := p.allocator.allocate(context.Background())
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = p.FetchHistoryBatch(ctx, domain.D1, map[string]time.Time{"AAPL": time.Now().Add(-time.Hour)})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}

	ch.mu.Lock()
	occupied := len(ch.symbols)
	ch.mu.Unlock()
	if occupied != 0 {
		t.Fatalf("channel still holds %d symbols", occupied)
	}
	lockVal, ok := p.historyLocks.Load(candleKey("AAPL", domain.D1))
	if !ok || !lockVal.(*sync.Mutex).TryLock() {
		t.Fatal("history lock was not released")
	}
}
