package tastytrade

import (
	"testing"
	"time"
)

func TestClassifyOpenTimeout(t *testing.T) {
	sent := time.Unix(1000, 0)
	after := sent.Add(time.Second)
	before := sent.Add(-time.Second)

	tests := []struct {
		name           string
		lastMsgAt      time.Time
		lastErrAt      time.Time
		openedReceived bool
		lateResponses  int64
		want           string
	}{
		{"network_silent zero", time.Time{}, time.Time{}, false, 0, "network_silent"},
		{"network_silent before sent", before, after, true, 3, "network_silent"},
		{"server_error", after, after, true, 2, "server_error"},
		{"partial_response", after, before, true, 2, "partial_response"},
		{"server_slow", after, time.Time{}, false, 1, "server_slow"},
		{"requests_ignored", after, time.Time{}, false, 0, "requests_ignored"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyOpenTimeout(tt.lastMsgAt, tt.lastErrAt, sent, tt.openedReceived, tt.lateResponses)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
