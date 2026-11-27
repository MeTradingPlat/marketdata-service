package tastytrade

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestDxLinkConn_Connect_ClosingDoesNotDial(t *testing.T) {
	var dialed atomic.Bool
	c := NewDxLinkConn(func() string {
		dialed.Store(true)
		return "ws://127.0.0.1:0"
	}, func() string { return "tok" })
	c.Close()

	if err := c.Connect(context.Background()); err == nil {
		t.Fatal("expected error when Connect called after Close")
	}
	if dialed.Load() {
		t.Fatal("expected Connect not to dial when closing")
	}
}

func TestDxLinkConn_Connect_CtxCanceledDuringHandshake(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		<-r.Context().Done()
	}))
	defer srv.Close()

	c := NewDxLinkConn(func() string { return "ws" + strings.TrimPrefix(srv.URL, "http") }, func() string { return "tok" })
	t.Cleanup(c.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if err := c.Connect(ctx); err == nil {
		t.Fatal("expected error when ctx expires during handshake")
	}
}
