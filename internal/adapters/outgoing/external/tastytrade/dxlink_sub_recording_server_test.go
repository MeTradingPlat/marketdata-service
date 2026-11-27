package tastytrade

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func startSubRecordingDxLinkServer(t *testing.T) (string, <-chan int64) {
	t.Helper()
	subCh := make(chan int64, 16)
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
			var env feedSubscriptionMessage
			if json.Unmarshal(raw, &env) != nil {
				continue
			}
			switch env.Type {
			case "SETUP":
				_ = conn.WriteJSON(map[string]any{"type": "SETUP", "channel": 0})
			case "AUTH":
				_ = conn.WriteJSON(map[string]any{"type": "AUTH_STATE", "channel": 0, "state": "AUTHORIZED"})
			case "CHANNEL_REQUEST":
				_ = conn.WriteJSON(map[string]any{"type": "CHANNEL_OPENED", "channel": env.Channel, "service": "FEED"})
			case "FEED_SETUP":
				_ = conn.WriteJSON(map[string]any{"type": "FEED_CONFIG", "channel": env.Channel})
			case "FEED_SUBSCRIPTION":
				for _, item := range env.Add {
					if item.Type == "Candle" && item.FromTime != nil {
						subCh <- *item.FromTime
					}
				}
			}
		}
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http"), subCh
}
