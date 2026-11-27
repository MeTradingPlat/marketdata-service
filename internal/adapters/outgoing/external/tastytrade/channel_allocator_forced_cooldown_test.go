package tastytrade

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestChannelAllocator_ForcedReconnectCooldown(t *testing.T) {
	orig := channelOpenTimeout
	channelOpenTimeout = 15 * time.Millisecond
	t.Cleanup(func() { channelOpenTimeout = orig })

	var n1, n2 atomic.Int32
	url1, close1 := startSelectiveAckDxLinkServer(t, &n1)
	url2, close2 := startSelectiveAckDxLinkServer(t, &n2)
	pc1 := newPooledConnection(newConnectedDxLinkConn(t, url1))
	pc2 := newPooledConnection(newConnectedDxLinkConn(t, url2))

	a := newChannelAllocator(nil, nil, nil, 2)
	a.connections = []*pooledConnection{pc1, pc2}

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := a.openChannelOn(ctx, pc1); !errors.Is(err, errChannelOpenTimeout) {
			t.Fatalf("pc1 attempt %d: %v", i+1, err)
		}
	}
	<-close1

	for i := 0; i < 3; i++ {
		if _, err := a.openChannelOn(ctx, pc2); !errors.Is(err, errChannelOpenTimeout) {
			t.Fatalf("pc2 attempt %d: %v", i+1, err)
		}
	}
	select {
	case <-close2:
		t.Fatal("expected second quarantine within 1m not to force reconnect")
	case <-time.After(50 * time.Millisecond):
	}

	a.mu.Lock()
	a.lastForcedReconnectAt = time.Now().Add(-forcedReconnectCooldown - time.Second)
	a.mu.Unlock()
	for i := 0; i < 3; i++ {
		_, _ = a.openChannelOn(ctx, pc2)
	}
	select {
	case <-close2:
	case <-time.After(time.Second):
		t.Fatal("expected forced reconnect after forcedReconnectCooldown elapsed")
	}
}
