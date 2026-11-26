package tastytrade

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestHealSilentSymbols_FailureCutoff(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	baseFactory, _ := newTestConnFactory(t, wsURL)
	var dials atomic.Int32
	factory := func(ctx context.Context) (*DxLinkConn, error) {
		if dials.Add(1) > 1 {
			return nil, errors.New("dial blocked")
		}
		return baseFactory(ctx)
	}
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.allocator = newChannelAllocator(factory, p.wireChannel, p.handleConnectionReconnect, 2)
	p.SetMaxChannelsPerConn(1)

	ctx := context.Background()
	now := time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC)
	p.lastLiveEventAtUnixNano.Store(time.Now().Add(-time.Second).UnixNano())
	for _, sym := range []string{"A", "B", "C", "D", "E"} {
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
	if got := p.HealSilentSymbols(ctx, now, 5); got != 0 {
		t.Fatalf("expected 0 healed on dial error, got %d", got)
	}
	if got := int(dials.Load()) - 1; got != silentHealMaxConsecutiveFailures {
		t.Fatalf("expected %d failure attempts before cutoff, got %d", silentHealMaxConsecutiveFailures, got)
	}
}
