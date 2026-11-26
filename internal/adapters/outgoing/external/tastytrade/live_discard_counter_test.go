package tastytrade

import (
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestSampledDiscardCounter_ThrottlesToOncePerMinute(t *testing.T) {
	var c sampledDiscardCounter
	base := time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC)
	c.record("AAPL", base, "msg")
	c.record("MSFT", base.Add(30*time.Second), "msg")
	if c.total() != 2 || c.lastLogNs.Load() != base.UnixNano() {
		t.Fatalf("total=%d lastLogNs=%d", c.total(), c.lastLogNs.Load())
	}
	t2 := base.Add(time.Minute)
	c.record("NVDA", t2, "msg")
	if c.total() != 3 || c.lastLogNs.Load() != t2.UnixNano() {
		t.Fatalf("after 1m: total=%d lastLogNs=%d", c.total(), c.lastLogNs.Load())
	}
}

func TestHandleLiveEvent_FutureAndDiscards(t *testing.T) {
	p := &CandlePool{
		current:    make(map[string]domain.Candle),
		lastClosed: make(map[string]domain.Candle),
		liveSubs:   make(map[string]func(domain.Candle)),
	}
	called := 0
	p.liveSubs["AAPL"] = func(domain.Candle) { called++ }

	now := time.Date(2026, 11, 25, 15, 10, 0, 0, time.UTC)
	p.handleLiveEventAt("AAPL", rawCandleEvent{Timestamp: now.Add(3 * time.Minute)}, now)
	if p.futureLiveDiscards.total() != 1 || p.lastLiveEventAtUnixNano.Load() != 0 || len(p.current) != 0 || len(p.lastNewBarAt) != 0 {
		t.Fatalf("future event touched state: discards=%d lastEvent=%d", p.futureLiveDiscards.total(), p.lastLiveEventAtUnixNano.Load())
	}

	p.dispatchClosed("AAPL", domain.Candle{Symbol: "AAPL", Timestamp: now})
	if p.incompleteLiveDiscards.total() != 1 || called != 0 {
		t.Fatalf("incomplete discard=%d called=%d", p.incompleteLiveDiscards.total(), called)
	}

	c := domain.Candle{Symbol: "AAPL", Timeframe: domain.M1, Timestamp: now, Open: 1, High: 1, Low: 1, Close: 1}
	p.lastClosed["AAPL"] = c
	p.currentMu.Lock()
	p.handleLateCorrection("AAPL", rawCandleEvent{Timestamp: now}, c, true)
	if p.lateUnchangedDiscards.total() != 1 || called != 0 {
		t.Fatalf("late unchanged discard=%d called=%d", p.lateUnchangedDiscards.total(), called)
	}
}
