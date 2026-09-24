package tastytrade

import (
	"testing"
	"time"
)

func dayVolumeEvent(symbol string, volume float64) rawTradeEvent {
	return rawTradeEvent{Symbol: symbol, DayVolume: &volume}
}

func TestTradeCollector_RepeatedTradesOfAKnownSymbolDoNotKeepTheBatchOpen(t *testing.T) {
	collector := newTradeCollector()
	collector.onTrade(dayVolumeEvent("AAPL", 100))
	time.Sleep(60 * time.Millisecond)

	collector.onTrade(dayVolumeEvent("AAPL", 150))

	if !collector.settled(2, 50*time.Millisecond) {
		t.Fatal("a repeated trade of a symbol already seen must not reset the quiet period")
	}
	if got := collector.result()["AAPL"]; got != 150 {
		t.Fatalf("AAPL = %d, want the latest 150", got)
	}
}

func TestTradeCollector_ANewSymbolRestartsTheQuietPeriod(t *testing.T) {
	collector := newTradeCollector()
	collector.onTrade(dayVolumeEvent("AAPL", 100))
	time.Sleep(60 * time.Millisecond)

	collector.onTrade(dayVolumeEvent("MSFT", 200))

	if collector.settled(3, 50*time.Millisecond) {
		t.Fatal("a symbol seen just now must keep the batch open for the quiet period")
	}
}

func TestTradeCollector_IsSettledOnceEveryExpectedSymbolArrived(t *testing.T) {
	collector := newTradeCollector()
	collector.onTrade(dayVolumeEvent("AAPL", 100))
	collector.onTrade(dayVolumeEvent("MSFT", 200))

	if !collector.settled(2, time.Hour) {
		t.Fatal("all expected symbols arrived, the batch must be settled without waiting")
	}
}
