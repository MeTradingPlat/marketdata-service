package main

import (
	"testing"
	"time"
)

func TestIsDayVolumeRefreshWindow(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		when time.Time
		want bool
	}{
		{"a weekday at 10am ET", time.Date(2026, 9, 22, 10, 0, 0, 0, loc), true},
		{"a weekday at 3:59am ET", time.Date(2026, 9, 22, 3, 59, 0, 0, loc), false},
		{"a weekday at 8pm ET", time.Date(2026, 9, 22, 20, 0, 0, 0, loc), false},
		{"a Saturday at noon ET", time.Date(2026, 9, 26, 12, 0, 0, 0, loc), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isDayVolumeRefreshWindow(c.when); got != c.want {
				t.Errorf("isDayVolumeRefreshWindow(%v) = %v, want %v", c.when, got, c.want)
			}
		})
	}
}
