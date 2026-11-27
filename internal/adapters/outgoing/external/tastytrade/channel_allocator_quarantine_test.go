package tastytrade

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestChannelAllocator_QuarantineAfterThreeTimeouts(t *testing.T) {
	orig := channelOpenTimeout
	channelOpenTimeout = 20 * time.Millisecond
	t.Cleanup(func() { channelOpenTimeout = orig })

	var connNum atomic.Int32
	wsURL, closeCh := startSelectiveAckDxLinkServer(t, &connNum)
	a := newChannelAllocator(func(ctx context.Context) (*DxLinkConn, error) {
		c := NewDxLinkConn(func() string { return wsURL }, func() string { return "tok" })
		return c, c.Connect(ctx)
	}, nil, nil, 2)
	t.Cleanup(func() {
		for _, pc := range a.drainAll() {
			pc.conn.Close()
		}
	})

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := a.allocate(ctx); !errors.Is(err, errChannelOpenTimeout) {
			t.Fatalf("attempt %d: expected errChannelOpenTimeout, got %v", i+1, err)
		}
	}
	select {
	case <-closeCh:
	case <-time.After(time.Second):
		t.Fatal("expected close frame on quarantined connection")
	}

	firstConn := a.connections[0].conn
	ch, err := a.allocate(ctx)
	if err != nil {
		t.Fatalf("4th allocate failed: %v", err)
	}
	if ch.channel.client == firstConn {
		t.Fatal("expected 4th allocate to use a different connection")
	}
}
