package tastytrade

import (
	"context"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestFetchHistory_SkipsLiveM1(t *testing.T) {
	p := newTestPool()
	p.liveSubs["AAPL"] = func(domain.Candle) {}

	got, err := p.FetchHistory(context.Background(), "AAPL", domain.M1, time.Now().Add(-time.Hour))
	if err != nil || got != nil {
		t.Fatalf("FetchHistory = (%v, %v), want (nil, nil)", got, err)
	}
}

func TestFetchHistoryWithWait_CancelledContext(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.allocator = newChannelAllocator(factory, p.wireChannel, p.handleConnectionReconnect, 1)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := p.FetchHistoryWithWait(ctx, "AAPL", domain.D1, time.Now().Add(-time.Hour), 20*time.Millisecond)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
