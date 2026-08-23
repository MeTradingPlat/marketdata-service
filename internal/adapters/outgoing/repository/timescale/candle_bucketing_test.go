package timescale

import (
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parsing %q: %v", s, err)
	}
	return ts
}

func TestAggregateIntoBuckets(t *testing.T) {
	raw := []domain.Candle{
		{Symbol: "HOWL", Timestamp: mustParse(t, "2026-08-23T10:07:00Z"), Open: 5, High: 9, Low: 4, Close: 8, Volume: 100, TradeCount: 3},
		{Symbol: "HOWL", Timestamp: mustParse(t, "2026-08-23T10:06:00Z"), Open: 3, High: 6, Low: 2, Close: 5, Volume: 50, TradeCount: 2},
		{Symbol: "HOWL", Timestamp: mustParse(t, "2026-08-23T10:04:00Z"), Open: 20, High: 21, Low: 18, Close: 19, Volume: 10, TradeCount: 1},
		{Symbol: "HOWL", Timestamp: mustParse(t, "2026-08-23T10:02:00Z"), Open: 15, High: 22, Low: 14, Close: 16, Volume: 20, TradeCount: 4},
	}

	got := aggregateIntoBuckets(raw, domain.M5, 5*time.Minute, 10)

	if len(got) != 2 {
		t.Fatalf("expected 2 buckets, got %d: %+v", len(got), got)
	}

	first := got[0]
	if !first.Timestamp.Equal(mustParse(t, "2026-08-23T10:05:00Z")) {
		t.Errorf("bucket[0] timestamp = %v, want 10:05:00Z", first.Timestamp)
	}
	if first.Open != 3 || first.Close != 8 || first.High != 9 || first.Low != 2 || first.Volume != 150 || first.TradeCount != 5 {
		t.Errorf("bucket[0] OHLCV = %+v, want open=3 high=9 low=2 close=8 volume=150 tradeCount=5", first)
	}
	if first.Timeframe != domain.M5 || first.Source != "aggregated" {
		t.Errorf("bucket[0] timeframe/source = %v/%v, want M5/aggregated", first.Timeframe, first.Source)
	}

	second := got[1]
	if !second.Timestamp.Equal(mustParse(t, "2026-08-23T10:00:00Z")) {
		t.Errorf("bucket[1] timestamp = %v, want 10:00:00Z", second.Timestamp)
	}
	if second.Open != 15 || second.Close != 19 || second.High != 22 || second.Low != 14 || second.Volume != 30 || second.TradeCount != 5 {
		t.Errorf("bucket[1] OHLCV = %+v, want open=15 high=22 low=14 close=19 volume=30 tradeCount=5", second)
	}
}

func TestAggregateIntoBucketsRespectsMax(t *testing.T) {
	raw := []domain.Candle{
		{Timestamp: mustParse(t, "2026-08-23T10:09:00Z"), Open: 1, High: 1, Low: 1, Close: 1},
		{Timestamp: mustParse(t, "2026-08-23T10:04:00Z"), Open: 1, High: 1, Low: 1, Close: 1},
		{Timestamp: mustParse(t, "2026-08-23T09:59:00Z"), Open: 1, High: 1, Low: 1, Close: 1},
	}

	got := aggregateIntoBuckets(raw, domain.M5, 5*time.Minute, 2)

	if len(got) != 2 {
		t.Fatalf("expected exactly 2 buckets (maxBuckets cap), got %d", len(got))
	}
}

func TestSourcePeriodOf(t *testing.T) {
	cases := []struct {
		source domain.Timeframe
		want   time.Duration
		wantOk bool
	}{
		{domain.M1, time.Minute, true},
		{domain.H1, time.Hour, true},
		{domain.D1, 0, false},
	}
	for _, tc := range cases {
		got, ok := sourcePeriodOf(tc.source)
		if got != tc.want || ok != tc.wantOk {
			t.Errorf("sourcePeriodOf(%v) = (%v, %v), want (%v, %v)", tc.source, got, ok, tc.want, tc.wantOk)
		}
	}
}
