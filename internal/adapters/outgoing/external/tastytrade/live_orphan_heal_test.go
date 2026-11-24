package tastytrade

import (
	"context"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestReattachOrphans_RespectsGraceAndLimit(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.allocator = newChannelAllocator(factory, p.wireChannel, p.handleConnectionReconnect, 1)

	ctx := context.Background()
	if err := p.WarmUp(ctx); err != nil {
		t.Fatalf("WarmUp: %v", err)
	}

	for _, sym := range []string{"AAA", "BBB", "CCC"} {
		p.liveSubs[sym] = func(domain.Candle) {}
		p.liveTicks[sym] = func(domain.Candle) {}
	}

	base := time.Date(2026, 11, 23, 15, 0, 0, 0, time.UTC)
	if got := p.ReattachOrphans(ctx, base, 10); got != 0 {
		t.Fatalf("first pass: expected 0, got %d", got)
	}
	if len(p.orphanSince) != 3 {
		t.Fatalf("expected 3 orphanSince marks, got %d", len(p.orphanSince))
	}
	if got := p.ReattachOrphans(ctx, base.Add(1*time.Minute), 10); got != 0 {
		t.Fatalf("before grace: expected 0, got %d", got)
	}

	if got := p.ReattachOrphans(ctx, base.Add(2*time.Minute+time.Second), 2); got != 2 {
		t.Fatalf("after grace with limit=2: expected 2, got %d", got)
	}
	pooled := p.pooledLiveSymbols()
	for _, sym := range []string{"AAA", "BBB"} {
		if _, ok := pooled[sym]; !ok {
			t.Fatalf("expected %s in pooledLiveSymbols after reattach", sym)
		}
	}
	if _, ok := pooled["CCC"]; ok {
		t.Fatal("expected CCC still outside pooledLiveSymbols when limit=2")
	}

	if got := p.ReattachOrphans(ctx, base.Add(5*time.Minute), 10); got != 1 {
		t.Fatalf("remaining pass: expected 1, got %d", got)
	}
	if _, ok := p.pooledLiveSymbols()["CCC"]; !ok {
		t.Fatal("expected CCC in pooledLiveSymbols after remaining pass")
	}
	if len(p.liveSubs) != 3 {
		t.Fatalf("expected liveSubs to preserve all 3 symbols, got %d", len(p.liveSubs))
	}
}
