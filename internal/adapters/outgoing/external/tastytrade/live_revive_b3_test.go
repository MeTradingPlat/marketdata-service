package tastytrade

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestReviveSilentLive_StopsAfter3FailuresAndIgnoresCanceledCtx(t *testing.T) {
	p := newTestPool()
	p.ensureActivityMaps()
	p.allocator = newChannelAllocator(func(context.Context) (*DxLinkConn, error) {
		return nil, errors.New("dial blocked")
	}, p.wireChannel, p.handleConnectionReconnect, 1)

	now := time.Now()
	for _, sym := range []string{"AAA", "BBB", "CCC", "DDD"} {
		p.liveSubs[sym] = func(domain.Candle) {}
		p.pendingRevive[sym] = reviveState{reconnectedAt: now.Add(-5 * time.Minute)}
	}
	p.ReviveSilentLive(context.Background(), now)
	attempted := 0
	for _, st := range p.pendingRevive {
		attempted += st.attempts
	}
	if attempted != 3 {
		t.Fatalf("expected 3 attempts before stopping, got %d", attempted)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	p.ReviveSilentLive(canceled, now.Add(time.Minute))
	afterCancel := 0
	for _, st := range p.pendingRevive {
		afterCancel += st.attempts
	}
	if afterCancel != attempted {
		t.Fatalf("canceled ctx should not increment attempts: before=%d after=%d", attempted, afterCancel)
	}
}

func TestStartReviveSilentLive_NoOverlap(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	p := newTestPool()
	p.ensureActivityMaps()
	p.allocator = newChannelAllocator(func(ctx context.Context) (*DxLinkConn, error) {
		close(started)
		<-release
		return nil, errors.New("done")
	}, p.wireChannel, p.handleConnectionReconnect, 1)
	now := time.Now()
	p.liveSubs["AAA"] = func(domain.Candle) {}
	p.pendingRevive["AAA"] = reviveState{reconnectedAt: now.Add(-5 * time.Minute)}

	if !p.StartReviveSilentLive(context.Background(), now) {
		t.Fatal("expected first StartReviveSilentLive to start")
	}
	<-started
	if p.StartReviveSilentLive(context.Background(), now) {
		t.Fatal("expected second StartReviveSilentLive to be skipped while running")
	}
	close(release)
}
