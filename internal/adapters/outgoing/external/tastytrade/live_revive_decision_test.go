package tastytrade

import (
	"testing"
	"time"
)

func TestShouldRevive(t *testing.T) {
	now := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	reconnect := now.Add(-5 * time.Minute)
	tests := []struct {
		name      string
		st        reviveState
		lastEvent time.Time
		want      bool
	}{
		{
			name:      "less than 3 min since reconnect",
			st:        reviveState{reconnectedAt: now.Add(-2 * time.Minute)},
			lastEvent: now.Add(-10 * time.Minute),
			want:      false,
		},
		{
			name:      "event after reconnect",
			st:        reviveState{reconnectedAt: reconnect},
			lastEvent: reconnect.Add(1 * time.Minute),
			want:      false,
		},
		{
			name:      "silent at least 3 min",
			st:        reviveState{reconnectedAt: reconnect},
			lastEvent: reconnect.Add(-1 * time.Minute),
			want:      true,
		},
		{
			name:      "max attempts reached",
			st:        reviveState{reconnectedAt: reconnect, attempts: 2},
			lastEvent: reconnect.Add(-1 * time.Minute),
			want:      false,
		},
		{
			name:      "retried less than 3 min ago",
			st:        reviveState{reconnectedAt: reconnect, attempts: 1, lastAttemptAt: now.Add(-1 * time.Minute)},
			lastEvent: reconnect.Add(-1 * time.Minute),
			want:      false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldRevive(tc.st, tc.lastEvent, now); got != tc.want {
				t.Fatalf("shouldRevive() = %v, want %v", got, tc.want)
			}
		})
	}
}
