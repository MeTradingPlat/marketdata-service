package tastytrade

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestChannelAllocator_FactoryFailure_CanceledSkipsQuarantined(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	c := newConnectedDxLinkConn(t, wsURL)
	pc := newPooledConnection(c)
	ch, err := c.OpenChannel(context.Background())
	if err != nil {
		t.Fatalf("OpenChannel: %v", err)
	}
	pc.addChannel(newPooledChannel(ch))
	pc.quarantinedUntil = time.Now().Add(10 * time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	boom := errors.New("dial canceled")
	a := newChannelAllocator(func(context.Context) (*DxLinkConn, error) {
		cancel()
		return nil, boom
	}, nil, nil, 2)
	a.connections = []*pooledConnection{pc}

	_, err = a.allocate(ctx)
	if !errors.Is(err, boom) {
		t.Fatalf("expected factory error, got %v", err)
	}
	if got := c.openChannelCount(); got != 1 {
		t.Fatalf("expected 1 channel on quarantined conn, got %d", got)
	}
}
