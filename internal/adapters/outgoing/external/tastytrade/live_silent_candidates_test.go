package tastytrade

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestSilentHealCandidates_IgnoresSiblingWithoutOwnTrade(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.allocator = newChannelAllocator(factory, p.wireChannel, p.handleConnectionReconnect, 1)

	ctx := context.Background()
	now := time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC)
	for _, sym := range []string{"MUTE", "ACTV"} {
		if err := p.SubscribeLive(ctx, sym, now, func(domain.Candle) {}, func(domain.Candle) {}); err != nil {
			t.Fatalf("SubscribeLive %s: %v", sym, err)
		}
	}
	p.noteNewBar("MUTE", now.Add(-6*time.Minute), now.Add(-6*time.Minute))
	p.noteNewBar("ACTV", now.Add(-30*time.Second), now.Add(-30*time.Second))

	if got := p.silentHealCandidates(now, 10); len(got) != 0 {
		t.Fatalf("expected no candidates without own trade, got %v", got)
	}
}

func TestSilentHealCandidates_RanksByDayVolumeBeforeLimit(t *testing.T) {
	p := newTestPool()
	p.liveDayVolume = make(map[string]liveDayVolumeEntry)
	now := time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC)

	vols := map[string]float64{"LOW": 100, "ZEBRA": 500, "ALPHA": 500, "HIGH": 1000}
	for sym, v := range vols {
		val := v
		p.liveSubs[sym] = func(domain.Candle) {}
		p.liveDayVolume[sym] = liveDayVolumeEntry{Regular: &val}
		p.noteNewBar(sym, now.Add(-6*time.Minute), now.Add(-6*time.Minute))
		p.noteTrade(sym, now.Add(-1*time.Minute))
	}

	got := p.silentHealCandidates(now, 3)
	want := []string{"HIGH", "ALPHA", "ZEBRA"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
