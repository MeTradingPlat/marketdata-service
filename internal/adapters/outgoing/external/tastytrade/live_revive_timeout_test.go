package tastytrade

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestStartReviveSilentLive_EnforcesTimeout(t *testing.T) {
	orig := reviveTimeout
	reviveTimeout = 40 * time.Millisecond
	t.Cleanup(func() { reviveTimeout = orig })

	var hasDeadline atomic.Bool
	done := make(chan error, 1)
	p := newTestPool()
	p.ensureActivityMaps()
	p.allocator = newChannelAllocator(func(ctx context.Context) (*DxLinkConn, error) {
		_, ok := ctx.Deadline()
		hasDeadline.Store(ok)
		<-ctx.Done()
		done <- ctx.Err()
		return nil, ctx.Err()
	}, p.wireChannel, p.handleConnectionReconnect, 1)

	now := time.Now()
	p.liveSubs["AAA"] = func(domain.Candle) {}
	p.pendingRevive["AAA"] = reviveState{reconnectedAt: now.Add(-5 * time.Minute)}

	if !p.StartReviveSilentLive(context.Background(), now) {
		t.Fatal("expected StartReviveSilentLive to start")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected DeadlineExceeded, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for StartReviveSilentLive timeout")
	}
	if !hasDeadline.Load() {
		t.Fatal("expected context with deadline passed to ReviveSilentLive")
	}
}
