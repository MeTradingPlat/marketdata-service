package domain

import (
	"testing"
	"time"
)

func TestIsNYSEHoliday_EdgeCases(t *testing.T) {
	tokyo, _ := time.LoadLocation("Asia/Tokyo")
	cases := []struct {
		name string
		day  time.Time
		want bool
	}{
		{"juneteenth before 2022", time.Date(2021, 6, 18, 12, 0, 0, 0, time.UTC), false},
		{"juneteenth 2021 sat", time.Date(2021, 6, 19, 12, 0, 0, 0, time.UTC), false},
		{"new year sunday observed mon", time.Date(2023, 1, 2, 12, 0, 0, 0, time.UTC), true},
		{"new year sat not observed fri", time.Date(2027, 12, 31, 12, 0, 0, 0, time.UTC), false},
		{"new year sat itself", time.Date(2028, 1, 1, 12, 0, 0, 0, time.UTC), false},
		{"uses day own zone", time.Date(2026, 1, 1, 1, 0, 0, 0, tokyo), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsNYSEHoliday(tc.day); got != tc.want {
				t.Fatalf("IsNYSEHoliday(%v) = %v, want %v", tc.day, got, tc.want)
			}
		})
	}
}
