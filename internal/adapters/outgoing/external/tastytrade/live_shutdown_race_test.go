package tastytrade

import (
	"context"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestHeal_SkipsWhileStopping(t *testing.T) {
	p := newTestPool()
	p.liveStopping = 1
	p.liveSubs["AAPL"] = func(domain.Candle) {}
	now := time.Now()
	if ok, failed := p.reattachOrphanSymbol(context.Background(), "AAPL", now); ok || failed {
		t.Fatalf("reattachOrphanSymbol = (%v,%v), want (false,false)", ok, failed)
	}
	if _, exists := p.orphanSince["AAPL"]; exists {
		t.Fatal("expected no orphanSince entry while stopping")
	}
	if ok, failed := p.reviveSilentSymbol(context.Background(), "AAPL", now); ok || failed {
		t.Fatalf("reviveSilentSymbol = (%v,%v), want (false,false)", ok, failed)
	}
	if _, exists := p.pendingRevive["AAPL"]; exists {
		t.Fatal("expected no pendingRevive entry while stopping")
	}
}

func TestHeal_EpochChangedBeforeRegister(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.allocator = newChannelAllocator(factory, p.wireChannel, p.handleConnectionReconnect, 1)
	if err := p.WarmUp(context.Background()); err != nil {
		t.Fatalf("WarmUp: %v", err)
	}
	p.liveSubs["AAPL"] = func(domain.Candle) {}
	now := time.Now()
	p.liveEpoch = 2
	if err := p.subscribeLiveAtEpoch(context.Background(), "AAPL", now, func(domain.Candle) {}, nil, 1); err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if _, ok := p.pooledLiveSymbols()["AAPL"]; ok {
		t.Fatal("expected AAPL released after epoch mismatch")
	}
}
