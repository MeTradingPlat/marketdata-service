package tastytrade

import (
	"context"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestSubscribeLive_RollbackOnSendError(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.allocator = newChannelAllocator(factory, p.wireChannel, p.handleConnectionReconnect, 1)

	ctx := context.Background()
	ch, err := p.allocator.allocate(ctx)
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	ch.channel.client = &DxLinkConn{}

	err = p.SubscribeLive(ctx, "NVDA", time.Now().Add(-time.Minute), func(domain.Candle) {}, func(domain.Candle) {})
	if err == nil {
		t.Fatal("expected SubscribeLive error on closed connection")
	}
	if !p.LiveSubscribed("NVDA") {
		t.Fatal("expected liveSubs to retain NVDA")
	}
	p.liveMu.Lock()
	_, hasTick := p.liveTicks["NVDA"]
	p.liveMu.Unlock()
	if hasTick {
		t.Fatal("expected liveTicks to roll back")
	}
	if p.hasLiveDispatch("NVDA") {
		t.Fatal("expected dispatch to roll back")
	}
	if p.liveChannelFor("NVDA") != nil {
		t.Fatal("expected channel occupancy to roll back")
	}
}

func TestSubscribeLive_Idempotent(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.allocator = newChannelAllocator(factory, p.wireChannel, p.handleConnectionReconnect, 1)

	ctx := context.Background()
	if err := p.SubscribeLive(ctx, "AAPL", time.Now(), func(domain.Candle) {}, func(domain.Candle) {}); err != nil {
		t.Fatalf("first SubscribeLive: %v", err)
	}
	firstCh := p.liveChannelFor("AAPL")
	if err := p.SubscribeLive(ctx, "AAPL", time.Now(), func(domain.Candle) {}, func(domain.Candle) {}); err != nil {
		t.Fatalf("second SubscribeLive: %v", err)
	}
	if got := p.liveChannelFor("AAPL"); got != firstCh {
		t.Fatal("expected idempotent SubscribeLive to keep same channel")
	}
}
