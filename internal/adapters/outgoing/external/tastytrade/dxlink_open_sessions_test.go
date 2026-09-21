package tastytrade

import (
	"context"
	"testing"
)

func TestDxLinkConn_OpenSessionsCounterFollowsAuthentication(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	before := openDxLinkSessions.Load()

	c := NewDxLinkConn(func() string { return wsURL }, func() string { return "test-token" })
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	if got := openDxLinkSessions.Load(); got != before+1 {
		t.Fatalf("after Connect: got %d open sessions, want %d", got, before+1)
	}

	c.Close()

	if got := openDxLinkSessions.Load(); got != before {
		t.Fatalf("after Close: got %d open sessions, want %d", got, before)
	}
}

func TestDxLinkConn_ClosingTwiceDoesNotUndercountSessions(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	before := openDxLinkSessions.Load()
	c := NewDxLinkConn(func() string { return wsURL }, func() string { return "test-token" })
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	c.Close()
	c.Close()

	if got := openDxLinkSessions.Load(); got != before {
		t.Fatalf("got %d open sessions, want %d", got, before)
	}
}
