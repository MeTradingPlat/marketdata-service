package tastytrade

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestChannelAllocator_FactoryFailure_QuarantinedFallback(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	c := newConnectedDxLinkConn(t, wsURL)
	pc := newPooledConnection(c)
	pc.quarantinedUntil = time.Now().Add(10 * time.Minute)
	boom := errors.New("dial failed")
	a := newChannelAllocator(func(context.Context) (*DxLinkConn, error) {
		return nil, boom
	}, nil, nil, 2)
	a.connections = []*pooledConnection{pc}

	ch, err := a.allocate(context.Background())
	if err != nil {
		t.Fatalf("expected quarantined fallback channel, got %v", err)
	}
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}
}

func TestChannelAllocator_FactoryFailure_NoRoomReturnsFactoryError(t *testing.T) {
	boom := errors.New("dial failed")
	a := newChannelAllocator(func(context.Context) (*DxLinkConn, error) {
		return nil, boom
	}, nil, nil, 2)

	_, err := a.allocate(context.Background())
	if !errors.Is(err, boom) {
		t.Fatalf("expected errors.Is(err, boom), got %v", err)
	}
}
