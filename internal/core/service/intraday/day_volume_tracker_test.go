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
