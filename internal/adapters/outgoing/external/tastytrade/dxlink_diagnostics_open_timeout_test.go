package tastytrade

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDxLinkConn_OpenChannel_TimeoutAndCancelDiagnostics(t *testing.T) {
	orig := channelOpenTimeout
	channelOpenTimeout = 30 * time.Millisecond
	t.Cleanup(func() { channelOpenTimeout = orig })

	wsURL, _, _, _ := startRecordingDxLinkServer(t, false)
	c := newConnectedDxLinkConn(t, wsURL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	_, err := c.OpenChannel(ctx)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
	if c.stats.opensCanceled.Load() != 1 {
		t.Fatalf("expected opensCanceled=1, got %d", c.stats.opensCanceled.Load())
	}

	_, err = c.OpenChannel(context.Background())
	if !errors.Is(err, errChannelOpenTimeout) {
		t.Fatalf("expected errChannelOpenTimeout, got %v", err)
	}
	msg := err.Error()
	for _, want := range []string{"connID", "channel", "open_channels", "elapsed", "last_message_age", "canceled", "timeouts", "cause"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected %q in error %q", want, msg)
		}
	}
}
