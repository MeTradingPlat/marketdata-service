package tastytrade

import "testing"

func TestCandlePool_LiveDayVolume_UnknownSymbolIsNotOk(t *testing.T) {
	p := NewCandlePool(nil, 0)

	_, ok := p.LiveDayVolume("AAPL")

	if ok {
		t.Fatal("a symbol with no Trade/TradeETH received yet must not be ok")
	}
}

func TestCandlePool_LiveDayVolume_TradeAndTradeETHUpdateIndependently(t *testing.T) {
	p := NewCandlePool(nil, 0)
	regular := 1000.0

	p.handleLiveTradeVolume(rawTradeEvent{Symbol: "AAPL", DayVolume: &regular})

	entry, ok := p.LiveDayVolume("AAPL")
	if !ok || *entry.Regular != regular || entry.Extended != nil {
		t.Fatalf("got %+v, ok=%v -- want only Regular set", entry, ok)
	}

	extended := 50.0
	p.handleLiveTradeETHVolume(rawTradeETHEvent{Symbol: "AAPL", ExtendedDayVolume: &extended})

	entry, ok = p.LiveDayVolume("AAPL")
	if !ok || *entry.Regular != regular || *entry.Extended != extended {
		t.Fatalf("got %+v, ok=%v -- want both Regular and Extended set", entry, ok)
	}
}

func TestCandlePool_LiveExtendedDayVolumes_OnlyReturnsSymbolsWithAKnownExtendedValue(t *testing.T) {
	p := NewCandlePool(nil, 0)
	extended := 512_000.0
	p.handleLiveTradeETHVolume(rawTradeETHEvent{Symbol: "AAPL", ExtendedDayVolume: &extended})
	regular := 9_000_000.0
	p.handleLiveTradeVolume(rawTradeEvent{Symbol: "MSFT", DayVolume: &regular})

	got := p.LiveExtendedDayVolumes([]string{"AAPL", "MSFT", "SPY"})

	if got["AAPL"] != 512_000 {
		t.Errorf("AAPL = %d, want 512000", got["AAPL"])
	}
	if _, ok := got["MSFT"]; ok {
		t.Errorf("MSFT has no TradeETH yet, must be absent, got %v", got["MSFT"])
	}
	if _, ok := got["SPY"]; ok {
		t.Errorf("SPY is unknown, must be absent, got %v", got["SPY"])
	}
}

func TestCandlePool_LiveDayVolume_ANilFieldIsIgnoredNotOverwritten(t *testing.T) {
	p := NewCandlePool(nil, 0)
	regular := 1000.0
	p.handleLiveTradeVolume(rawTradeEvent{Symbol: "AAPL", DayVolume: &regular})

	p.handleLiveTradeVolume(rawTradeEvent{Symbol: "AAPL", DayVolume: nil})

	entry, ok := p.LiveDayVolume("AAPL")
	if !ok || entry.Regular == nil || *entry.Regular != regular {
		t.Fatalf("got %+v, ok=%v -- a nil DayVolume must not erase the last known value", entry, ok)
	}
}
