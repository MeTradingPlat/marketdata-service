package tastytrade

import (
	"context"
	"testing"
)

func TestDxLinkConn_OpenChannel_FailsWhenNotConnected(t *testing.T) {
	c := NewDxLinkConn(func() string { return "" }, func() string { return "" })
	if _, err := c.OpenChannel(context.Background()); err == nil {
		t.Fatal("expected OpenChannel to fail before any Connect()")
	}
}

func TestDxLinkConn_Send_FailsWithoutConnection(t *testing.T) {
	c := NewDxLinkConn(func() string { return "" }, func() string { return "" })
	if err := c.send(keepaliveMessage{Type: "KEEPALIVE", Channel: 0}); err == nil {
		t.Fatal("expected send to fail without an open connection")
	}
}

func TestDxLinkConn_Connect_CompletesHandshakeAndAuthenticates(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	c := newConnectedDxLinkConn(t, wsURL)

	if !c.Connected() {
		t.Fatal("expected Connected() to be true after a successful handshake")
	}
}

func TestDxLinkConn_OpenChannel_SucceedsOnceConnected(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	c := newConnectedDxLinkConn(t, wsURL)

	ch, err := c.OpenChannel(context.Background())
	if err != nil {
		t.Fatalf("expected OpenChannel to succeed once authenticated, got: %v", err)
	}
	if ch == nil {
		t.Fatal("expected a non-nil channel")
	}
}

// Close es un cierre INTENCIONAL (ver el comentario en dxlink_conn.go) --
// confirma que deja la conexion inutilizable y no solo "parece cerrada".
func TestDxLinkConn_Close_ResetsStateAndPreventsFurtherUse(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	c := newConnectedDxLinkConn(t, wsURL)

	c.Close()

	if c.Connected() {
		t.Fatal("expected Connected() to be false after Close()")
	}
	if _, err := c.OpenChannel(context.Background()); err == nil {
		t.Fatal("expected OpenChannel to fail after Close()")
	}
	select {
	case <-c.Done():
	default:
		t.Fatal("expected Done() to be closed after Close()")
	}
}
