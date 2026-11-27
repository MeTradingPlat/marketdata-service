package tastytrade

import "testing"

func TestParseCandleBatch_NonFiniteStringsDoNotSplitRecord(t *testing.T) {
	data := []interface{}{
		"AAPL{=1d}", float64(1700000000000),
		"NaN", "Infinity", "-Infinity", float64(101.5),
		"NaN", "NaN", "NaN", float64(eventFlagSnapshotEnd),
	}

	events := parseCandleBatch(data)
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(events), events)
	}
	ev := events[0]
	if ev.Symbol != "AAPL{=1d}" {
		t.Errorf("symbol = %q, want AAPL{=1d}", ev.Symbol)
	}
	if ev.EventFlags != eventFlagSnapshotEnd {
		t.Errorf("eventFlags = %d, want %d", ev.EventFlags, eventFlagSnapshotEnd)
	}
	if ev.Open != nil || ev.High != nil || ev.Low != nil || ev.Volume != nil || ev.VWAP != nil {
		t.Errorf("expected non-finite fields to be nil, got Open=%v High=%v Low=%v Volume=%v VWAP=%v",
			ev.Open, ev.High, ev.Low, ev.Volume, ev.VWAP)
	}
	if ev.Close == nil || *ev.Close != 101.5 {
		t.Errorf("close = %v, want 101.5", ev.Close)
	}
}

func TestParseCandleBatch_MultipleSymbolsWithNonFiniteAndShortRecord(t *testing.T) {
	data := []interface{}{
		"AAPL{=1m}", float64(1700000000000), "NaN", float64(10), float64(9), float64(9.5), float64(100), "NaN", "NaN", float64(eventFlagSnapshotEnd),
		"MSFT{=1m}", float64(1700000060000), float64(200), float64(201), float64(199), float64(200.5),
	}

	events := parseCandleBatch(data)
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].Symbol != "AAPL{=1m}" || events[0].EventFlags != eventFlagSnapshotEnd {
		t.Errorf("first event = %+v", events[0])
	}
	if events[1].Symbol != "MSFT{=1m}" || events[1].Close == nil || *events[1].Close != 200.5 {
		t.Errorf("second event = %+v", events[1])
	}
}
