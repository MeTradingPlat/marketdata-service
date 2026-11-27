package tastytrade

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestReattachOrphans_CanceledCtxDoesNotResetOrphanSince(t *testing.T) {
	p := newTestPool()
	p.allocator = newChannelAllocator(func(context.Context) (*DxLinkConn, error) {
		return nil, errors.New("dial blocked")
	}, p.wireChannel, p.handleConnectionReconnect, 1)

	origSince := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	p.ensureActivityMaps()
	p.liveSubs["AAA"] = func(domain.Candle) {}
	p.orphanSince["AAA"] = origSince

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	now := origSince.Add(orphanReattachGrace + time.Minute)
	if got := p.ReattachOrphans(ctx, now, 10); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
	if !p.orphanSince["AAA"].Equal(origSince) {
		t.Fatalf("expected orphanSince unchanged (%v), got %v", origSince, p.orphanSince["AAA"])
	}
}
