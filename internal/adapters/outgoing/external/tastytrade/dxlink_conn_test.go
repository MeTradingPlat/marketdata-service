package tastytrade

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
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

func TestDxLinkConn_Send_FailedWriteClosesConnection(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	c := newConnectedDxLinkConn(t, wsURL)

	c.mu.Lock()
	_ = c.conn.UnderlyingConn().Close()
	c.mu.Unlock()

	err := c.send(keepaliveMessage{Type: "KEEPALIVE", Channel: 0})
	if err == nil {
		t.Fatal("expected send to fail after closing underlying conn")
	}
	if c.Connected() {
		t.Fatal("expected Connected() to be false after failed write")
	}
}

func TestDxLinkConn_NotifyHandshakeFailure_SessionSaturated(t *testing.T) {
	c := NewDxLinkConn(func() string { return "" }, func() string { return "" })
	c.notifyHandshakeFailure(&websocket.CloseError{Code: websocket.CloseNormalClosure, Text: "Bye"})
	if c.sessionSaturated.Load() {
		t.Fatal("expected sessionSaturated to be false when close 1000 occurs without handshake")
	}

	c.mu.Lock()
	c.handshakeDone = make(chan error, 1)
	c.mu.Unlock()
	c.notifyHandshakeFailure(&websocket.CloseError{Code: websocket.CloseNormalClosure, Text: "Bye"})
	if !c.sessionSaturated.Load() {
		t.Fatal("expected sessionSaturated to be true when close 1000 occurs with handshake in progress")
	}
}

func TestDxLinkConn_PerformReconnect_BlocksTimeout(t *testing.T) {
	origTimeout := reconnectStepTimeout
	reconnectStepTimeout = 50 * time.Millisecond
	defer func() { reconnectStepTimeout = origTimeout }()

	t.Run("Connect blocks", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}))
		defer srv.Close()

		wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
		c := NewDxLinkConn(func() string { return wsURL }, func() string { return "token" })

		start := time.Now()
		err := c.performReconnect(context.Background())
		if err == nil {
			t.Fatal("expected performReconnect to return an error when Connect blocks")
		}
		if time.Since(start) > 2*time.Second {
			t.Fatalf("performReconnect hung (%v), expected timeout near 50ms", time.Since(start))
		}
		if c.Connected() {
			t.Fatal("expected Connected() to be false")
		}
	})

	t.Run("onReconnect blocks", func(t *testing.T) {
		wsURL := startFakeDxLinkServer(t)
		c := NewDxLinkConn(func() string { return wsURL }, func() string { return "token" })
		c.OnReconnect(func(ctx context.Context, _ *DxLinkConn) {
			<-ctx.Done()
		})

		start := time.Now()
		err := c.performReconnect(context.Background())
		if err == nil {
			t.Fatal("expected performReconnect to return an error when onReconnect blocks")
		}
		if time.Since(start) > 2*time.Second {
			t.Fatalf("performReconnect hung (%v), expected timeout near 50ms", time.Since(start))
		}
		if c.Connected() {
			t.Fatal("expected Connected() to be false")
		}
	})
}
