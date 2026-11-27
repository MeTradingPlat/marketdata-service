package tastytrade

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
)

func startRecordingDxLinkServer(t *testing.T, ackChannel bool) (string, *[]channelCancelMessage, *sync.Mutex, <-chan channelCancelMessage) {
	t.Helper()
	var mu sync.Mutex
	var msgs []channelCancelMessage
	cancelCh := make(chan channelCancelMessage, 4)
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var env channelCancelMessage
			if json.Unmarshal(raw, &env) != nil {
				continue
			}
			mu.Lock()
			msgs = append(msgs, env)
			mu.Unlock()
			switch env.Type {
			case "SETUP":
				_ = conn.WriteJSON(map[string]any{"type": "SETUP", "channel": 0})
			case "AUTH":
				_ = conn.WriteJSON(map[string]any{"type": "AUTH_STATE", "channel": 0, "state": "AUTHORIZED"})
			case "CHANNEL_REQUEST":
				if ackChannel {
					_ = conn.WriteJSON(map[string]any{"type": "CHANNEL_OPENED", "channel": env.Channel, "service": "FEED"})
				}
			case "CHANNEL_CANCEL":
				cancelCh <- env
			}
		}
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http"), &msgs, &mu, cancelCh
}
