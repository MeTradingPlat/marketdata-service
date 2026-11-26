package tastytrade

import (
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestIsLateCandleEvent(t *testing.T) {
	t10 := time.Date(2026, 11, 25, 15, 10, 0, 0, time.UTC)
	t11 := t10.Add(time.Minute)
	cases := []struct {
		name    string
		evTS    time.Time
		prev    domain.Candle
		last    domain.Candle
		exists  bool
		hasLast bool
		want    bool
	}{
		{"before current forming", t10, domain.Candle{Timestamp: t11}, domain.Candle{}, true, false, true},
		{"equal to current forming", t11, domain.Candle{Timestamp: t11}, domain.Candle{}, true, false, false},
		{"no current, equal to last closed", t10, domain.Candle{}, domain.Candle{Timestamp: t10}, false, true, true},
		{"no current, after last closed", t11, domain.Candle{}, domain.Candle{Timestamp: t10}, false, true, false},
		{"neither current nor last", t10, domain.Candle{}, domain.Candle{}, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev := rawCandleEvent{Timestamp: tc.evTS}
			if got := isLateCandleEvent(ev, tc.prev, tc.last, tc.exists, tc.hasLast); got != tc.want {
				t.Fatalf("isLateCandleEvent() = %v, want %v", got, tc.want)
			}
		})
	}
}
