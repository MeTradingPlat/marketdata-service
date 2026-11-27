package tastytrade

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestDxLinkConn_CloseReason_FirstWinsAndResetsOnPublish(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			var env channelCancelMessage
			if err := conn.ReadJSON(&env); err != nil {
				return
			}
			switch env.Type {
			case "SETUP":
				_ = conn.WriteJSON(map[string]any{"type": "SETUP", "channel": 0})
			case "AUTH":
				_ = conn.WriteJSON(map[string]any{"type": "AUTH_STATE", "channel": 0, "state": "AUTHORIZED"})
				_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseGoingAway, "bye"))
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	c := newConnectedDxLinkConn(t, wsURL)
	deadline := time.Now().Add(time.Second)
	for c.closeReasonText() == "" && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	reason := c.closeReasonText()
	if !strings.Contains(reason, "read_error") || !strings.Contains(reason, "1001") {
		t.Fatalf("expected read_error with 1001, got %q", reason)
	}
	c.noteCloseReason("second_reason")
	if c.closeReasonText() != reason {
		t.Fatalf("expected first reason to win, got %q", c.closeReasonText())
	}
	c.Close()
	if c.closeReasonText() != reason {
		t.Fatalf("expected Close not to overwrite reason, got %q", c.closeReasonText())
	}
}
