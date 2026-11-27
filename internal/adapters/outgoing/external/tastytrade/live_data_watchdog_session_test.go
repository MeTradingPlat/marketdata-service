package tastytrade

import (
	"testing"
	"time"
)

func TestIsExtendedSessionET(t *testing.T) {
	cases := []struct {
		name string
		now  time.Time
		want bool
	}{
		{"normal 03:59 ET", time.Date(2026, 11, 25, 8, 59, 0, 0, time.UTC), false},
		{"normal 04:00 ET", time.Date(2026, 11, 25, 9, 0, 0, 0, time.UTC), true},
		{"normal 19:59 ET", time.Date(2026, 11, 26, 0, 59, 0, 0, time.UTC), true},
		{"normal 20:00 ET", time.Date(2026, 11, 26, 1, 0, 0, 0, time.UTC), false},
		{"thanksgiving 12:00 ET", time.Date(2026, 11, 26, 17, 0, 0, 0, time.UTC), false},
		{"early close 16:59 ET", time.Date(2026, 11, 27, 21, 59, 0, 0, time.UTC), true},
		{"early close 17:00 ET", time.Date(2026, 11, 27, 22, 0, 0, 0, time.UTC), false},
		{"saturday 12:00 ET", time.Date(2026, 11, 28, 17, 0, 0, 0, time.UTC), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isExtendedSessionET(tc.now); got != tc.want {
				t.Fatalf("isExtendedSessionET(%v) = %v, want %v", tc.now, got, tc.want)
			}
		})
	}
}
