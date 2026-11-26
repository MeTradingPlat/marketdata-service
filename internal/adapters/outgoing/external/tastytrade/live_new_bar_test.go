package tastytrade

import (
	"testing"
	"time"
)

func TestHandleLiveEvent_LastNewBarAtIgnoresOlderReplay(t *testing.T) {
	p := newTestPool()
	base := time.Date(2026, 11, 25, 15, 5, 0, 0, time.UTC)
	recorded := base.Add(-10 * time.Minute)
	p.activityMu.Lock()
	p.ensureActivityMaps()
	p.lastBarTS["AAPL"] = base
	p.lastNewBarAt["AAPL"] = recorded
	p.activityMu.Unlock()

	c1, c2 := 100.0, 101.0
	p.handleLiveEvent("AAPL", rawCandleEvent{Symbol: "AAPL{=1m}", Timestamp: base.Add(-2 * time.Minute), Close: &c1})
	p.activityMu.Lock()
	afterReplay := p.lastNewBarAt["AAPL"]
	p.activityMu.Unlock()
	if !afterReplay.Equal(recorded) {
		t.Fatalf("expected lastNewBarAt to ignore older replay: got %v, want %v", afterReplay, recorded)
	}

	p.handleLiveEvent("AAPL", rawCandleEvent{Symbol: "AAPL{=1m}", Timestamp: base.Add(time.Minute), Close: &c2})
	p.activityMu.Lock()
	afterNew := p.lastNewBarAt["AAPL"]
	p.activityMu.Unlock()
	if !afterNew.After(recorded) {
		t.Fatal("expected lastNewBarAt to advance on newer bar")
	}
}
