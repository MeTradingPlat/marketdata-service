package tastytrade

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestFetchHistoryBatch_SkipsLiveDispatchAndUnlocks(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.allocator = newChannelAllocator(factory, p.wireChannel, p.handleConnectionReconnect, 1)

	liveID := p.registerDispatch("NVDA", domain.M1, "live", func(rawCandleEvent) {})
	res, err := p.FetchHistoryBatch(context.Background(), domain.M1, map[string]time.Time{
		"NVDA": time.Now().Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("FetchHistoryBatch: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("expected empty result, got %d", len(res))
	}
	if !p.hasLiveDispatch("NVDA") || p.dispatch[candleKey("NVDA", domain.M1)].id != liveID {
		t.Fatal("expected live dispatch to remain untouched")
	}
	if p.liveChannelFor("NVDA") != nil {
		t.Fatal("expected NVDA not to occupy any channel")
	}
	lockVal, ok := p.historyLocks.Load(candleKey("NVDA", domain.M1))
	if !ok || !lockVal.(*sync.Mutex).TryLock() {
		t.Fatal("expected historyLock for NVDA to be released")
	}
	lockVal.(*sync.Mutex).Unlock()
}
