package tastytrade

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestChannelAllocator_CeilingQuarantinedReservesKeys(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	c := newConnectedDxLinkConn(t, wsURL)
	pc := newPooledConnection(c)
	pc.quarantinedUntil = time.Now().Add(10 * time.Minute)
	a := newChannelAllocator(func(context.Context) (*DxLinkConn, error) {
		return nil, errors.New("unused")
	}, nil, nil, 1)
	a.connections = []*pooledConnection{pc}

	ch1, err := a.allocate(context.Background(), "AAPL{=1m}")
	if err != nil {
		t.Fatalf("open at ceiling: %v", err)
	}
	if !channelHasKey(ch1, "AAPL{=1m}") {
		t.Fatal("expected AAPL reserved on new channel")
	}

	ch2, err := a.allocate(context.Background(), "MSFT{=1m}")
	if err != nil {
		t.Fatalf("reuse at ceiling: %v", err)
	}
	if ch2 != ch1 || !channelHasKey(ch2, "MSFT{=1m}") {
		t.Fatal("expected MSFT reserved on reused channel")
	}
}
