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

func TestDxLinkConn_CloseDuringHandshake_ReturnsFastAndCleansUp(t *testing.T) {
	inHandshake := make(chan struct{}, 1)
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_, _, _ = conn.ReadMessage()
		select {
		case inHandshake <- struct{}{}:
		default:
		}
		<-r.Context().Done()
	}))
	defer srv.Close()

	before := openDxLinkSessions.Load()
	c := NewDxLinkConn(func() string { return "ws" + strings.TrimPrefix(srv.URL, "http") }, func() string { return "tok" })
	errCh := make(chan error, 1)
	go func() { errCh <- c.Connect(context.Background()) }()

	<-inHandshake
	start := time.Now()
	c.Close()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error from Connect when Close() called during handshake")
		}
		if time.Since(start) > time.Second {
			t.Fatalf("Connect took too long to abort on Close(): %v", time.Since(start))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Connect to return after Close()")
	}
	if c.Connected() {
		t.Fatal("expected Connected() false after Close()")
	}
	if got := openDxLinkSessions.Load(); got != before {
		t.Fatalf("expected openDxLinkSessions=%d, got %d", before, got)
	}
}
