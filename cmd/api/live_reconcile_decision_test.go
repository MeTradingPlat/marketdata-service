package main

import "testing"

func TestShouldSkipReconcileRetry(t *testing.T) {
	cases := []struct {
		name                                   string
		attempted, rolloutDone, live, subscribed bool
		want                                   bool
	}{
		{"never attempted, rollout still running: skip", false, false, false, false, true},
		{"never attempted, rollout finished: retry", false, true, false, false, false},
		{"attempted, live and subscribed: skip", true, true, true, true, true},
		{"attempted, live but not subscribed (silent death): retry", true, true, true, false, false},
		{"attempted, failed (not live): retry", true, true, false, false, false},
		{"attempted and failed, rollout still running: retry", true, false, false, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := shouldSkipReconcileRetry(c.attempted, c.rolloutDone, c.live, c.subscribed)
			if got != c.want {
				t.Errorf("shouldSkipReconcileRetry(%v, %v, %v, %v) = %v, want %v",
					c.attempted, c.rolloutDone, c.live, c.subscribed, got, c.want)
			}
		})
	}
}
