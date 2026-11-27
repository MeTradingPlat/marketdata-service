package intraday

import (
	"testing"
	"time"
)

func TestPhaseAt_EarlyClose(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		at   time.Time
		want SessionPhase
	}{
		{"early close 12:59 regular", time.Date(2026, 11, 27, 12, 59, 0, 0, loc), PhaseRegular},
		{"early close 13:00 post", time.Date(2026, 11, 27, 13, 0, 0, 0, loc), PhasePostMarket},
		{"early close 16:59 post", time.Date(2026, 11, 27, 16, 59, 0, 0, loc), PhasePostMarket},
		{"early close 17:00 closed", time.Date(2026, 11, 27, 17, 0, 0, 0, loc), PhaseClosed},
		{"normal day 13:00 regular", time.Date(2026, 11, 30, 13, 0, 0, 0, loc), PhaseRegular},
		{"normal day 03:59 closed", time.Date(2026, 11, 30, 3, 59, 0, 0, loc), PhaseClosed},
		{"normal day 04:00 pre", time.Date(2026, 11, 30, 4, 0, 0, 0, loc), PhasePreMarket},
		{"weekend closed", time.Date(2026, 11, 28, 11, 0, 0, 0, loc), PhaseClosed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PhaseAt(tt.at); got != tt.want {
				t.Fatalf("PhaseAt(%v) = %v, want %v", tt.at, got, tt.want)
			}
		})
	}
}
