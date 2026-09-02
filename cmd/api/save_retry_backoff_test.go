package main

import (
	"testing"
	"time"
)

func TestNextSaveRetryInterval(t *testing.T) {
	cases := []struct {
		name      string
		current   time.Duration
		succeeded bool
		want      time.Duration
	}{
		{"success resets to base", time.Minute, true, saveRetryBaseInterval},
		{"nothing pending resets to base", saveRetryMaxInterval, true, saveRetryBaseInterval},
		{"first failure doubles", saveRetryBaseInterval, false, 30 * time.Second},
		{"second failure doubles again", 30 * time.Second, false, time.Minute},
		{"failure caps at max", saveRetryMaxInterval, false, saveRetryMaxInterval},
		{"failure just under cap clamps to cap", saveRetryMaxInterval - time.Second, false, saveRetryMaxInterval},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := nextSaveRetryInterval(c.current, c.succeeded)
			if got != c.want {
				t.Errorf("nextSaveRetryInterval(%v, %v) = %v, want %v", c.current, c.succeeded, got, c.want)
			}
		})
	}
}
