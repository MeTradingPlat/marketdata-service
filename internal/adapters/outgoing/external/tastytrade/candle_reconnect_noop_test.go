package tastytrade

import (
	"context"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestCandlePool_HandleConnectionReconnect_NoRetiredTouchesNothing(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := NewCandlePool(factory, 2)
	t.Cleanup(p.CloseAllConnections)

	ctx := context.Background()
	if err := p.SubscribeLive(ctx, "AAA", time.Now().Add(-time.Minute), func(domain.Candle) {}, nil); err != nil {
		t.Fatalf("SubscribeLive: %v", err)
	}
	ts := time.Now().Truncate(time.Minute)
	p.currentMu.Lock()
	p.current["AAA"] = domain.Candle{Symbol: "AAA", Timestamp: ts}
	p.currentMu.Unlock()

	p.handleConnectionReconnect(ctx, p.allocator.connections[0])

	p.currentMu.Lock()
	got, ok := p.current["AAA"]
	p.currentMu.Unlock()
	if !ok || !got.Timestamp.Equal(ts) {
		t.Fatalf("expected current AAA untouched, got ok=%v ts=%v", ok, got.Timestamp)
	}
	if p.liveChannelFor("AAA") == nil {
		t.Fatal("expected AAA live channel preserved")
	}
}
