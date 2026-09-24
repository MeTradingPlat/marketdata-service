package intraday

import (
	"testing"
	"time"
)

func TestDayVolumeTracker_GetReturnsFalseWhenUnknown(t *testing.T) {
	tracker := NewDayVolumeTracker()

	_, ok := tracker.Get("AAPL")

	if ok {
		t.Fatal("expected ok=false for a symbol never updated")
	}
}

func TestDayVolumeTracker_UpdateThenGetReturnsTheValue(t *testing.T) {
	tracker := NewDayVolumeTracker()
	day := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)

	tracker.Update(day, map[string]int64{"AAPL": 9_491_383})

	got, ok := tracker.Get("AAPL")
	if !ok || got != 9_491_383 {
		t.Fatalf("got %v ok=%v, want 9491383 true", got, ok)
	}
}

func TestDayVolumeTracker_APartialRoundKeepsSymbolsMissingFromIt(t *testing.T) {
	tracker := NewDayVolumeTracker()
	day := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	tracker.Update(day, map[string]int64{"AAPL": 100, "SPY": 200})

	tracker.Update(day, map[string]int64{"AAPL": 150})

	aapl, _ := tracker.Get("AAPL")
	spy, ok := tracker.Get("SPY")
	if aapl != 150 {
		t.Fatalf("AAPL = %d, want 150 (updated)", aapl)
	}
	if !ok || spy != 200 {
		t.Fatalf("SPY = %d ok=%v, want 200 true (kept from the earlier round)", spy, ok)
	}
}

func TestDayVolumeTracker_ANewDayResetsEverything(t *testing.T) {
	tracker := NewDayVolumeTracker()
	day1 := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	tracker.Update(day1, map[string]int64{"AAPL": 100})

	tracker.Update(day2, map[string]int64{"SPY": 200})

	if _, ok := tracker.Get("AAPL"); ok {
		t.Fatal("AAPL from yesterday should not survive a day change")
	}
	spy, ok := tracker.Get("SPY")
	if !ok || spy != 200 {
		t.Fatalf("SPY = %d ok=%v, want 200 true", spy, ok)
	}
}

func TestDayVolumeTracker_BoundariesAreKeptPerDayAndResetOnANewDay(t *testing.T) {
	tracker := NewDayVolumeTracker()
	day := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)
	tracker.SetBoundary(day, PreMarketEnd, map[string]int64{"AAPL": 480_000})

	before, okBefore := tracker.Boundary("AAPL", PreMarketEnd)
	tracker.Update(day.AddDate(0, 0, 1), map[string]int64{"AAPL": 10})
	_, okAfter := tracker.Boundary("AAPL", PreMarketEnd)

	if !okBefore || before != 480_000 || okAfter {
		t.Fatalf("before=%d/%v after ok=%v, want 480000/true and false", before, okBefore, okAfter)
	}
}

func TestPhaseAt(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	at := func(day, hour, minute int) time.Time { return time.Date(2026, 9, day, hour, minute, 0, 0, loc) }
	cases := map[string]struct {
		when time.Time
		want SessionPhase
	}{
		"before 4am":         {at(23, 3, 59), PhaseClosed},
		"pre-market":         {at(23, 4, 0), PhasePreMarket},
		"one minute to open": {at(23, 9, 29), PhasePreMarket},
		"open":               {at(23, 9, 30), PhaseRegular},
		"close":              {at(23, 16, 0), PhasePostMarket},
		"after 8pm":          {at(23, 20, 0), PhaseClosed},
		"saturday":           {at(26, 10, 0), PhaseClosed},
	}
	for name, tc := range cases {
		if got := PhaseAt(tc.when); got != tc.want {
			t.Errorf("%s: PhaseAt = %v, want %v", name, got, tc.want)
		}
	}
}
