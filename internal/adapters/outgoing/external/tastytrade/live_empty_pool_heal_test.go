package tastytrade

import (
	"context"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func newEmptyHealPool(t *testing.T) *CandlePool {
	t.Helper()
	factory, _ := newTestConnFactory(t, startFakeDxLinkServer(t))
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.allocator = newChannelAllocator(factory, p.wireChannel, p.handleConnectionReconnect, 1)
	return p
}

func TestReattachOrphans_RecoversFromPoolWithoutConnections(t *testing.T) {
	p := newEmptyHealPool(t)
	p.liveSubs["AAA"] = func(domain.Candle) {}
	p.liveTicks["AAA"] = func(domain.Candle) {}

	base := time.Date(2026, 11, 23, 15, 0, 0, 0, time.UTC)
	p.ReattachOrphans(context.Background(), base, 10)
	got := p.ReattachOrphans(context.Background(), base.Add(orphanReattachGrace+time.Second), 10)

	if got != 1 {
		t.Fatalf("expected 1 reattached from an empty pool, got %d", got)
	}
}

func TestCanSubscribeLive(t *testing.T) {
	p := newEmptyHealPool(t)
	if !p.CanSubscribeLive() {
		t.Fatal("empty pool must allow subscribing, allocate opens a connection")
	}
	if err := p.WarmUp(context.Background()); err != nil {
		t.Fatalf("WarmUp: %v", err)
	}
	if !p.CanSubscribeLive() {
		t.Fatal("connected pool must allow subscribing")
	}
	for _, pc := range p.allocator.connections {
		pc.conn.Close()
	}
	if p.CanSubscribeLive() {
		t.Fatal("pool with only dead connections must wait for their reconnection")
	}
}
