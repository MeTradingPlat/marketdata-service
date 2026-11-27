package tastytrade

import (
	"context"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestReviveSilentSymbol_NoRoomKeepsOldSubscriptionAndCountsFailure(t *testing.T) {
	wsURL, rec := startSubFeedRecordingServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.ensureActivityMaps()
	p.allocator = newChannelAllocator(factory, p.wireChannel, p.handleConnectionReconnect, 1)
	p.SetMaxChannelsPerConn(1)

	ctx := context.Background()
	syms := []string{"S1", "S2", "S3", "S4"}
	now := time.Date(2026, 11, 26, 15, 10, 0, 0, time.UTC)
	for _, sym := range syms {
		if err := p.SubscribeLive(ctx, sym, now, func(domain.Candle) {}, nil); err != nil {
			t.Fatalf("SubscribeLive %s: %v", sym, err)
		}
		p.activityMu.Lock()
		p.pendingRevive[sym] = reviveState{reconnectedAt: now.Add(-5 * time.Minute)}
		p.activityMu.Unlock()
	}
	oldCh := p.liveChannelFor("S1")
	before := len(rec.snapshot())
	p.ReviveSilentLive(ctx, now)
	for _, s := range rec.snapshot()[before:] {
		if s.Action == "remove" {
			t.Fatalf("unexpected remove when no new channel available: %+v", s)
		}
	}
	attempts := 0
	for _, sym := range syms {
		if p.liveChannelFor(sym) != oldCh {
			t.Fatalf("expected %s to stay on old channel", sym)
		}
		attempts += p.pendingRevive[sym].attempts
	}
	if attempts != reviveMaxConsecutiveFailures {
		t.Fatalf("expected %d failed attempts before stopping, got %d", reviveMaxConsecutiveFailures, attempts)
	}
}
