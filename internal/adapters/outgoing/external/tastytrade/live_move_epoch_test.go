package tastytrade

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestReviveSilentSymbol_EpochChangeAfterReserveReleasesAndSkipsIO(t *testing.T) {
	wsURL, rec := startSubFeedRecordingServer(t)
	baseFactory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.ensureActivityMaps()
	var dials atomic.Int32
	p.allocator = newChannelAllocator(func(ctx context.Context) (*DxLinkConn, error) {
		conn, err := baseFactory(ctx)
		if err == nil && dials.Add(1) == 2 {
			p.liveMu.Lock()
			p.liveEpoch++
			p.liveMu.Unlock()
		}
		return conn, err
	}, p.wireChannel, p.handleConnectionReconnect, 2)
	p.SetMaxChannelsPerConn(1)

	ctx := context.Background()
	now := time.Date(2026, 11, 26, 15, 0, 0, 0, time.UTC)
	if err := p.SubscribeLive(ctx, "NVDA", now, func(domain.Candle) {}, nil); err != nil {
		t.Fatalf("SubscribeLive: %v", err)
	}
	rec.waitCount(t, 3)
	oldCh := p.liveChannelFor("NVDA")
	p.activityMu.Lock()
	p.pendingRevive["NVDA"] = reviveState{reconnectedAt: now.Add(-5 * time.Minute)}
	p.activityMu.Unlock()

	before := len(rec.snapshot())
	p.ReviveSilentLive(ctx, now)
	if len(rec.snapshot()) != before {
		t.Fatalf("expected no subscribe/unsubscribe on epoch change, got %+v", rec.snapshot()[before:])
	}
	if p.liveChannelFor("NVDA") != oldCh {
		t.Fatal("expected NVDA to remain on old channel")
	}
	for _, ch := range p.allocator.allChannels() {
		if ch != oldCh {
			ch.mu.Lock()
			n := len(ch.symbols)
			ch.mu.Unlock()
			if n != 0 {
				t.Fatalf("expected reserved key released on new channel, got %d keys", n)
			}
		}
	}
}
