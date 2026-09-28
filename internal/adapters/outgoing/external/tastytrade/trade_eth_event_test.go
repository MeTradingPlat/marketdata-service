package tastytrade

import "testing"

func TestParseTradeETHBatch_ParsesEachSymbolsExtendedDayVolume(t *testing.T) {
	data := []interface{}{"AAPL", 12345.0, "MSFT", 6789.0}

	events := parseTradeETHBatch(data)

	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].Symbol != "AAPL" || *events[0].ExtendedDayVolume != 12345.0 {
		t.Fatalf("unexpected first event: %+v", events[0])
	}
	if events[1].Symbol != "MSFT" || *events[1].ExtendedDayVolume != 6789.0 {
		t.Fatalf("unexpected second event: %+v", events[1])
	}
}

func TestParseTradeETHBatch_SkipsRecordWithoutASymbol(t *testing.T) {
	data := []interface{}{"", 100.0, "AAPL", 200.0}

	events := parseTradeETHBatch(data)

	if len(events) != 1 || events[0].Symbol != "AAPL" {
		t.Fatalf("got %+v, want only the AAPL record", events)
	}
}

func TestParseTradeETHRecord_NilWhenTheFieldIsMissing(t *testing.T) {
	ev, ok := parseTradeETHRecord([]interface{}{"AAPL"})

	if !ok || ev.ExtendedDayVolume != nil {
		t.Fatalf("got %+v, ok=%v -- want ok=true and a nil ExtendedDayVolume", ev, ok)
	}
}
