package tastytrade

import (
	"context"
	"testing"
	"time"
)

func TestChannelAllocator_QuarantineOrderAtCeiling(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	c1 := newConnectedDxLinkConn(t, wsURL)
	c2 := newConnectedDxLinkConn(t, wsURL)
	pc1 := newPooledConnection(c1)
	pc2 := newPooledConnection(c2)

	now := time.Now()
	pc1.quarantineStrikes = 3
	pc1.quarantinedUntil = now.Add(10 * time.Minute)
	pc2.quarantineStrikes = 1
	pc2.quarantinedUntil = now.Add(10 * time.Minute)

	a := newChannelAllocator(nil, nil, nil, 2)
	a.connections = []*pooledConnection{pc1, pc2}

	ch2, err := a.allocate(context.Background())
	if err != nil {
		t.Fatalf("allocate at ceiling: %v", err)
	}
	if ch2.channel.client != c2 {
		t.Fatal("expected fewer-strikes connection c2 for new channel")
	}

	rawCh1, err := c1.OpenChannel(context.Background())
	if err != nil {
		t.Fatalf("OpenChannel c1: %v", err)
	}
	pc1.addChannel(newPooledChannel(rawCh1))

	pc2.mu.Lock()
	pc2.quarantineStrikes = 3
	pc2.quarantinedUntil = now.Add(5 * time.Minute)
	pc2.mu.Unlock()

	got := a.findRoomExcludingFiltered(nil, true)
	if got != ch2 {
		t.Fatal("expected earlier-quarantinedUntil connection c2 on tie-breaker")
	}
}
