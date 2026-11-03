package tastytrade

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func startDisconnectBeforeAuthDxLinkServer(t *testing.T) string {
	t.Helper()
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var env struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(msg, &env); err != nil {
				continue
			}
			if env.Type == "SETUP" {
				_ = conn.WriteJSON(map[string]any{"type": "SETUP", "channel": 0})
				closeMsg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Bye")
				_ = conn.WriteMessage(websocket.CloseMessage, closeMsg)
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

func TestDxLinkConn_FailedConnectClose(t *testing.T) {
	wsURL := startDisconnectBeforeAuthDxLinkServer(t)
	before := openDxLinkSessions.Load()

	c := NewDxLinkConn(func() string { return wsURL }, func() string { return "test-token" })
	err := c.Connect(context.Background())
	if err == nil {
		t.Fatal("expected non-nil error from Connect, got nil")
	}

	c.Close()

	if !c.closing.Load() {
		t.Fatal("expected closing to be true after Close so reconnectLoop exits instead of reconnecting")
	}

	if got := openDxLinkSessions.Load(); got != before {
		t.Fatalf("openDxLinkSessions mismatch: got %d, want %d", got, before)
	}
}
