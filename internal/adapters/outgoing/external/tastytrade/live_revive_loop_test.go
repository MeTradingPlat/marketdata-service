package tastytrade

import (
	"context"
	"testing"
	"time"
)

func TestDropExpiredRevive_RemovesExpiredOutsideExtendedHours(t *testing.T) {
	now := time.Now()
	p := &CandlePool{
		lastEventAt:   make(map[string]time.Time),
		pendingRevive: make(map[string]reviveState),
	}
	p.pendingRevive["EXPIRED"] = reviveState{reconnectedAt: now.Add(-31 * time.Minute)}
	p.lastEventAt["EXPIRED"] = now.Add(-31 * time.Minute)
	p.pendingRevive["RECENT"] = reviveState{reconnectedAt: now.Add(-1 * time.Minute)}
	p.lastEventAt["RECENT"] = now.Add(-40 * time.Minute)

	p.DropExpiredRevive(now)

	if _, ok := p.pendingRevive["EXPIRED"]; ok {
		t.Fatal("expected the expired entry to be dropped")
	}
	if _, ok := p.pendingRevive["RECENT"]; !ok {
		t.Fatal("expected the recent entry to be kept")
	}
}

func TestReviveSilentLive_DropsExpired(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := &CandlePool{
		allocator:     newChannelAllocator(factory, nil, nil, 10),
		lastEventAt:   make(map[string]time.Time),
		pendingRevive: make(map[string]reviveState),
	}
	now := time.Now()
	p.pendingRevive["EXPIRED"] = reviveState{reconnectedAt: now.Add(-31 * time.Minute)}
	p.lastEventAt["EXPIRED"] = now.Add(-31 * time.Minute)

	p.ReviveSilentLive(context.Background(), now)

	if _, ok := p.pendingRevive["EXPIRED"]; ok {
		t.Fatal("expected the expired entry to be dropped by ReviveSilentLive")
	}
}
