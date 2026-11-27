package tastytrade

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDxLinkConn_SequentialIDAndTimeoutErrorDetails(t *testing.T) {
	c1 := NewDxLinkConn(nil, nil)
	c2 := NewDxLinkConn(nil, nil)
	if c2.id <= c1.id {
		t.Fatalf("expected sequential connIDs, got c1=%d c2=%d", c1.id, c2.id)
	}

	orig := channelOpenTimeout
	channelOpenTimeout = 15 * time.Millisecond
	t.Cleanup(func() { channelOpenTimeout = orig })

	wsURL, _, _, _ := startRecordingDxLinkServer(t, false)
	c := newConnectedDxLinkConn(t, wsURL)
	_, err := c.OpenChannel(context.Background())
	if !errors.Is(err, errChannelOpenTimeout) {
		t.Fatalf("expected errChannelOpenTimeout, got %v", err)
	}
	if !strings.Contains(err.Error(), "connID") || !strings.Contains(err.Error(), "open_channels") {
		t.Fatalf("expected connID and open_channels in error: %v", err)
	}
}
