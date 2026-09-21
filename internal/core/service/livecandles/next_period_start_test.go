package livecandles

import (
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestNextPeriodStart(t *testing.T) {
	cases := map[string]struct {
		start time.Time
		tf    domain.Timeframe
		want  time.Time
	}{
		"M1":  {time.Date(2026, 9, 21, 14, 5, 0, 0, time.UTC), domain.M1, time.Date(2026, 9, 21, 14, 6, 0, 0, time.UTC)},
		"M5":  {time.Date(2026, 9, 21, 14, 5, 0, 0, time.UTC), domain.M5, time.Date(2026, 9, 21, 14, 10, 0, 0, time.UTC)},
		"M15": {time.Date(2026, 9, 21, 14, 0, 0, 0, time.UTC), domain.M15, time.Date(2026, 9, 21, 14, 15, 0, 0, time.UTC)},
		"H1":  {time.Date(2026, 9, 21, 23, 0, 0, 0, time.UTC), domain.H1, time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)},
		"D1":  {time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC), domain.D1, time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC)},
		"W1":  {time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC), domain.W1, time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)},
		"MO1": {time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), domain.MO1, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
	}
	for name, tc := range cases {
		if got := NextPeriodStart(tc.start, tc.tf); !got.Equal(tc.want) {
			t.Errorf("%s: got %v, want %v", name, got, tc.want)
		}
	}
}
