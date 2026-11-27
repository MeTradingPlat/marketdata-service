package tastytrade

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gorilla/websocket"
)

func startSelectiveAckDxLinkServer(t *testing.T, connNum *atomic.Int32) (string, <-chan struct{}) {
	t.Helper()
	closeCh := make(chan struct{}, 4)
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := connNum.Add(1)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetCloseHandler(func(int, string) error {
			select {
			case closeCh <- struct{}{}:
			default:
			}
			return nil
		})
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var env struct {
				Type    string `json:"type"`
				Channel int    `json:"channel"`
			}
			if json.Unmarshal(raw, &env) != nil {
				continue
			}
			switch env.Type {
			case "SETUP":
				_ = conn.WriteJSON(map[string]any{"type": "SETUP", "channel": 0})
			case "AUTH":
				_ = conn.WriteJSON(map[string]any{"type": "AUTH_STATE", "channel": 0, "state": "AUTHORIZED"})
			case "CHANNEL_REQUEST":
				if n > 1 {
					_ = conn.WriteJSON(map[string]any{"type": "CHANNEL_OPENED", "channel": env.Channel, "service": "FEED"})
				}
			case "FEED_SETUP":
				_ = conn.WriteJSON(map[string]any{"type": "FEED_CONFIG", "channel": env.Channel})
			}
		}
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http"), closeCh
}
