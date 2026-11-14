package tastytrade

import (
	"context"
	"testing"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestPooledConnection_TakeLiveSymbols(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	a := newChannelAllocator(factory, nil, nil, 1)
	ch, err := a.allocate(context.Background())
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	ch.occupy(candleKey("AAPL", domain.M1))
	pc := a.connections[0]
	if !pc.owns(ch) {
		t.Fatal("expected owns true before take")
	}
	syms := pc.takeLiveSymbols()
	if len(syms) != 1 || syms[0] != "AAPL" {
		t.Fatalf("expected [AAPL], got %v", syms)
	}
	if len(pc.channels) != 0 || pc.owns(ch) {
		t.Fatal("expected channels empty and owns false")
	}
}

func TestCandlePool_ClaimLiveChannel_OccupiesAndOwns(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	p.allocator = newChannelAllocator(factory, nil, nil, 10)
	ctx := context.Background()
	ch1, err := p.claimLiveChannel(ctx, "AAA")
	if err != nil || !p.allocator.owns(ch1) {
		t.Fatalf("claim AAA: %v", err)
	}
	if syms := ch1.liveSymbols(); len(syms) != 1 || syms[0] != "AAA" {
		t.Fatalf("expected [AAA], got %v", syms)
	}
	if taken := p.allocator.connections[0].takeLiveSymbols(); len(taken) != 1 || taken[0] != "AAA" {
		t.Fatalf("expected [AAA], got %v", taken)
	}
	ch2, err := p.claimLiveChannel(ctx, "BBB")
	if err != nil || !p.allocator.owns(ch2) || ch2 == ch1 {
		t.Fatalf("claim BBB: %v, same: %v", err, ch2 == ch1)
	}
}
