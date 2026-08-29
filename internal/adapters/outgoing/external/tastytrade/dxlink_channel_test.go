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

// TestDxLinkChannelOpen_ConnectionDeathUnblocksWaiter reproduce en vivo el
// bloqueo del 2026-08-29: un canal nuevo pide CHANNEL_REQUEST y el socket
// muere antes de que CHANNEL_OPENED/FEED_CONFIG lleguen. Sin connDone,
// open() se queda esperando para siempre porque el ctx del llamador (el
// barrido nocturno) no tiene timeout propio.
func TestDxLinkChannelOpen_ConnectionDeathUnblocksWaiter(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		// Nunca responde CHANNEL_OPENED/FEED_CONFIG -- simula la conexion
		// muriendo a mitad del handshake del canal.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	dialConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dialing test server: %v", err)
	}

	c := NewDxLinkConn(func() string { return wsURL }, func() string { return "token" })
	c.conn = dialConn
	c.authenticated = true
	c.channels = make(map[int]*dxLinkChannel)
	c.connDone = make(chan struct{})

	ch := newDxLinkChannel(1, c)
	c.channels[1] = ch

	errCh := make(chan error, 1)
	go func() { errCh <- ch.open(context.Background()) }()

	// Deja que el CHANNEL_REQUEST realmente salga antes de matar la conexion.
	time.Sleep(50 * time.Millisecond)
	c.cleanup()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected an error once the connection died mid-handshake, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ch.open never returned after the connection died -- deadlock reproduced")
	}
}

func TestDxLinkConnDone_ReturnsClosedChannelWhenNotConnected(t *testing.T) {
	c := NewDxLinkConn(func() string { return "" }, func() string { return "" })
	select {
	case <-c.Done():
	default:
		t.Fatal("Done() should return an already-closed channel when there is no live connection")
	}
}
