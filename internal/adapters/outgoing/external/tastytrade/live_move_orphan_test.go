package tastytrade

import (
	"context"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestMoveLiveSymbol_SubscribeFailureBecomesOrphanAndReattachRecovers(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.ensureActivityMaps()
	p.allocator = newChannelAllocator(factory, p.wireChannel, p.handleConnectionReconnect, 2)
	p.SetMaxChannelsPerConn(2)

	ctx := context.Background()
	now := time.Date(2026, 11, 26, 15, 0, 0, 0, time.UTC)
	if err := p.SubscribeLive(ctx, "NVDA", now, func(domain.Candle) {}, nil); err != nil {
		t.Fatalf("SubscribeLive: %v", err)
	}
	oldCh := p.liveChannelFor("NVDA")
	newCh, err := p.allocator.allocateExcluding(ctx, oldCh)
	if err != nil {
		t.Fatalf("allocateExcluding: %v", err)
	}
	origClient := newCh.channel.client
	newCh.channel.client = &DxLinkConn{}

	p.activityMu.Lock()
	p.pendingRevive["NVDA"] = reviveState{reconnectedAt: now.Add(-5 * time.Minute)}
	p.activityMu.Unlock()
	p.ReviveSilentLive(ctx, now)
	if p.liveChannelFor("NVDA") != nil {
		t.Fatal("expected NVDA to have no channel after subscribe failure")
	}

	newCh.channel.client = origClient
	p.ReattachOrphans(ctx, now, 10)
	if got := p.ReattachOrphans(ctx, now.Add(orphanReattachGrace+time.Second), 10); got != 1 {
		t.Fatalf("expected ReattachOrphans to recover 1 symbol, got %d", got)
	}
	if p.liveChannelFor("NVDA") == nil {
		t.Fatal("expected NVDA to have a channel after ReattachOrphans")
	}
}
