package tastytrade

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestFetchHistory_AllocateFailureAndCancelledCleanup(t *testing.T) {
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.allocator = newChannelAllocator(func(context.Context) (*DxLinkConn, error) {
		return nil, errors.New("boom")
	}, p.wireChannel, p.handleConnectionReconnect, 1)

	if _, err := p.FetchHistory(context.Background(), "AAPL", domain.D1, time.Now().Add(-time.Hour)); err == nil {
		t.Fatal("expected FetchHistory to fail on allocate error")
	}

	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p.allocator = newChannelAllocator(factory, p.wireChannel, p.handleConnectionReconnect, 1)
	ch, err := p.allocator.allocate(context.Background())
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = p.FetchHistory(ctx, "AAPL", domain.D1, time.Now().Add(-time.Hour))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}

	ch.mu.Lock()
	occupied := len(ch.symbols)
	ch.mu.Unlock()
	if occupied != 0 {
		t.Fatalf("channel still holds %d symbols", occupied)
	}
	if lockVal, ok := p.historyLocks.Load(candleKey("AAPL", domain.D1)); ok {
		if !lockVal.(*sync.Mutex).TryLock() {
			t.Fatal("history lock was not released")
		}
		lockVal.(*sync.Mutex).Unlock()
	}
}
