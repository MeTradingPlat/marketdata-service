package tastytrade

import (
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestMergeCandle_SetsObservedAtFromLastEvent(t *testing.T) {
	t1 := time.Date(2026, 11, 27, 15, 0, 1, 0, time.UTC)
	t2 := time.Date(2026, 11, 27, 15, 0, 5, 0, time.UTC)
	ts := time.Date(2026, 11, 27, 15, 0, 0, 0, time.UTC)
	close1, close2 := 100.0, 101.0

	c1 := mergeCandle(domain.Candle{}, rawCandleEvent{
		Symbol:     "AAPL",
		Timestamp:  ts,
		Close:      &close1,
		ObservedAt: t1,
	}, "AAPL", domain.M1)
	if !c1.ObservedAt.Equal(t1) {
		t.Fatalf("after first merge ObservedAt = %v, want %v", c1.ObservedAt, t1)
	}

	c2 := mergeCandle(c1, rawCandleEvent{
		Symbol:     "AAPL",
		Timestamp:  ts,
		Close:      &close2,
		ObservedAt: t2,
	}, "AAPL", domain.M1)
	if !c2.ObservedAt.Equal(t2) {
		t.Fatalf("after second merge ObservedAt = %v, want %v", c2.ObservedAt, t2)
	}
}

func TestCandleValuesEqual_IgnoresObservedAt(t *testing.T) {
	ts := time.Date(2026, 11, 27, 15, 0, 0, 0, time.UTC)
	a := domain.Candle{
		Timestamp:  ts,
		Open:       100,
		High:       102,
		Low:        99,
		Close:      101,
		Volume:     50,
		VWAP:       100.5,
		ObservedAt: ts.Add(time.Second),
	}
	b := a
	b.ObservedAt = ts.Add(10 * time.Second)
	if !candleValuesEqual(a, b) {
		t.Fatal("candleValuesEqual must ignore ObservedAt")
	}
}
