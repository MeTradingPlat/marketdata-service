package tastytrade

import (
	"context"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestReviveSilentSymbol_MovesToDistinctChannel(t *testing.T) {
	wsURL, rec := startSubFeedRecordingServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.ensureActivityMaps()
	p.allocator = newChannelAllocator(factory, p.wireChannel, p.handleConnectionReconnect, 2)
	p.SetMaxChannelsPerConn(2)

	lastClosed := time.Date(2026, 11, 26, 15, 0, 0, 0, time.UTC)
	ctx := context.Background()
	if err := p.SubscribeLive(ctx, "NVDA", lastClosed.Add(5*time.Minute), func(domain.Candle) {}, nil); err != nil {
		t.Fatalf("SubscribeLive: %v", err)
	}
	rec.waitCount(t, 3)
	oldCh := p.liveChannelFor("NVDA")
	p.currentMu.Lock()
	p.lastClosed["NVDA"] = domain.Candle{Timestamp: lastClosed}
	p.currentMu.Unlock()
	now := lastClosed.Add(10 * time.Minute)
	p.activityMu.Lock()
	p.pendingRevive["NVDA"] = reviveState{reconnectedAt: now.Add(-5 * time.Minute)}
	p.activityMu.Unlock()

	p.ReviveSilentLive(ctx, now)
	newCh := p.liveChannelFor("NVDA")
	if newCh == nil || newCh == oldCh {
		t.Fatalf("expected NVDA on distinct channel, old=%v new=%v", oldCh, newCh)
	}
	rec.waitCount(t, 6)
	var gotRemove, gotAdd bool
	for _, s := range rec.snapshot() {
		if s.Channel == oldCh.channel.id && s.Action == "remove" && s.Symbol == "NVDA{=1m}" {
			gotRemove = true
		}
		if s.Channel == newCh.channel.id && s.Action == "add" && s.Symbol == "NVDA{=1m}" && s.FromTime == lastClosed.UnixMilli() {
			gotAdd = true
		}
	}
	if !gotRemove || !gotAdd {
		t.Fatalf("expected remove on %d and add on %d with fromTime=%d, got %+v", oldCh.channel.id, newCh.channel.id, lastClosed.UnixMilli(), rec.snapshot())
	}
}
