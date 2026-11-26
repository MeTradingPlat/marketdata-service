package tastytrade

import (
	"context"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestRegisterDispatch_DoesNotOverwriteLive(t *testing.T) {
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)

	liveCalled := false
	liveID := p.registerDispatch("NVDA", domain.M1, "live", func(rawCandleEvent) { liveCalled = true })
	if liveID == 0 {
		t.Fatal("expected non-zero live dispatch id")
	}
	if got := p.registerDispatch("NVDA", domain.M1, "history-batch", func(rawCandleEvent) {}); got != 0 {
		t.Fatalf("expected 0 when overwriting live dispatch, got %d", got)
	}
	p.unregisterDispatchIfCurrent("NVDA", domain.M1, liveID, "history-batch")
	p.routeEvent(rawCandleEvent{Symbol: "NVDA{=1m}"})
	if !liveCalled {
		t.Fatal("expected live handler to remain active")
	}
}

func TestFetchHistoryBatch_ExcludesLiveM1(t *testing.T) {
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.liveSubs["NVDA"] = func(domain.Candle) {}
	liveID := p.registerDispatch("NVDA", domain.M1, "live", func(rawCandleEvent) {})

	res, err := p.FetchHistoryBatch(context.Background(), domain.M1, map[string]time.Time{
		"NVDA": time.Now().Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("FetchHistoryBatch: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("expected empty result for live-only M1 batch, got %d", len(res))
	}
	if !p.hasLiveDispatch("NVDA") || p.dispatch[candleKey("NVDA", domain.M1)].id != liveID {
		t.Fatal("expected NVDA live dispatch to remain untouched")
	}
}
