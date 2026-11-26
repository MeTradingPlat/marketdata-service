package tastytrade

import (
	"context"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestStopAllLive_FlushesAndClearsState(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.allocator = newChannelAllocator(factory, p.wireChannel, p.handleConnectionReconnect, 1)

	var closed []domain.Candle
	ctx := context.Background()
	err := p.SubscribeLive(ctx, "AAPL", time.Now(), func(c domain.Candle) { closed = append(closed, c) }, func(domain.Candle) {})
	if err != nil {
		t.Fatalf("SubscribeLive: %v", err)
	}
	ts := time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC)
	p.handleLiveEvent("AAPL", rawCandleEvent{
		Symbol: "AAPL", Timestamp: ts,
		Open: f(100), High: f(101), Low: f(99), Close: f(100.5), Volume: f(500),
	})

	p.StopAllLive(ctx)
	if p.LiveSubscribed("AAPL") {
		t.Fatal("expected AAPL to be unsubscribed")
	}
	if p.hasLiveDispatch("AAPL") {
		t.Fatal("expected AAPL dispatch to be removed")
	}
	if len(closed) != 1 || closed[0].Symbol != "AAPL" {
		t.Fatalf("expected forming candle to flush, got %+v", closed)
	}
}
