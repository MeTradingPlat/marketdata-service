package tastytrade

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestHealSilentSymbols_Budget(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.allocator = newChannelAllocator(factory, p.wireChannel, p.handleConnectionReconnect, 2)
	p.SetMaxChannelsPerConn(2)

	ctx := context.Background()
	now := time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC)
	p.lastLiveEventAtUnixNano.Store(time.Now().Add(-time.Second).UnixNano())
	for _, sym := range []string{"A", "B", "C", "D"} {
		if err := p.SubscribeLive(ctx, sym, now, func(domain.Candle) {}, func(domain.Candle) {}); err != nil {
			t.Fatalf("SubscribeLive %s: %v", sym, err)
		}
		p.noteNewBar(sym, now.Add(-6*time.Minute), now.Add(-6*time.Minute))
		p.noteTrade(sym, now.Add(-1*time.Minute))
	}
	oldCh := p.liveChannelFor("A")
	for i := 0; i < channelCapacity; i++ {
		oldCh.occupy(fmt.Sprintf("F%03d", i))
	}
	if got := p.HealSilentSymbols(ctx, now, 2); got != 2 {
		t.Fatalf("expected 2 healed within budget, got %d", got)
	}
	moved := 0
	for _, sym := range []string{"A", "B", "C", "D"} {
		if p.liveChannelFor(sym) != oldCh {
			moved++
		}
	}
	if moved != 2 {
		t.Fatalf("expected 2 moved symbols, got %d", moved)
	}
}
