package tastytrade

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func startCloseRecordingDxLinkServer(t *testing.T) (string, <-chan int) {
	t.Helper()
	codes := make(chan int, 4)
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetCloseHandler(func(code int, _ string) error {
			codes <- code
			return nil
		})
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var env struct {
				Type string `json:"type"`
			}
			_ = json.Unmarshal(msg, &env)
			switch env.Type {
			case "SETUP":
				_ = conn.WriteJSON(map[string]any{"type": "SETUP", "channel": 0})
			case "AUTH":
				_ = conn.WriteJSON(map[string]any{"type": "AUTH_STATE", "channel": 0, "state": "AUTHORIZED"})
			}
		}
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http"), codes
}

func TestDxLinkConn_CloseSendsANormalClosureFrame(t *testing.T) {
	wsURL, codes := startCloseRecordingDxLinkServer(t)
	c := NewDxLinkConn(func() string { return wsURL }, func() string { return "test-token" })
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	c.Close()

	select {
	case code := <-codes:
		if code != websocket.CloseNormalClosure {
			t.Fatalf("close code = %d, want %d", code, websocket.CloseNormalClosure)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the server never received a close frame")
	}
}

func TestCloseConnections_ClosesEveryConnectionAndReturnsPromptly(t *testing.T) {
	var conns []*pooledConnection
	var allCodes []<-chan int
	for i := 0; i < 5; i++ {
		wsURL, codes := startCloseRecordingDxLinkServer(t)
		c := NewDxLinkConn(func() string { return wsURL }, func() string { return "test-token" })
		if err := c.Connect(context.Background()); err != nil {
			t.Fatalf("Connect failed: %v", err)
		}
		conns = append(conns, newPooledConnection(c))
		allCodes = append(allCodes, codes)
	}

	start := time.Now()
	closeConnections(conns, 5*time.Second)

	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("closing took %v", elapsed)
	}
	for i, codes := range allCodes {
		select {
		case <-codes:
		case <-time.After(3 * time.Second):
			t.Fatalf("connection %d never received a close frame", i)
		}
	}
}
