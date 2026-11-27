package domain

import (
	"testing"
	"time"
)

func TestIsNYSEEarlyClose_EdgeCases(t *testing.T) {
	cases := []struct {
		name string
		day  time.Time
		want bool
	}{
		{"thanksgiving 2026", time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC), false},
		{"black friday 2026", time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC), true},
		{"jul 3 2026 is holiday", time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC), false},
		{"dec 24 2027 is holiday", time.Date(2027, 12, 24, 12, 0, 0, 0, time.UTC), false},
		{"dec 24 2028 sunday", time.Date(2028, 12, 24, 12, 0, 0, 0, time.UTC), false},
		{"jul 3 2028 monday", time.Date(2028, 7, 3, 12, 0, 0, 0, time.UTC), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsNYSEEarlyClose(tc.day); got != tc.want {
				t.Fatalf("IsNYSEEarlyClose(%v) = %v, want %v", tc.day, got, tc.want)
			}
		})
	}
}
