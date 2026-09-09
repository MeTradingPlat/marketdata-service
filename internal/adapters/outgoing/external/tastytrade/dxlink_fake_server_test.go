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

// startFakeDxLinkServer levanta un servidor WS local que completa el
// handshake real de dxLink (SETUP -> AUTH -> AUTHORIZED, y
// CHANNEL_REQUEST -> CHANNEL_OPENED -> FEED_SETUP -> FEED_CONFIG para
// cualquier canal) -- suficiente para que DxLinkConn.Connect/OpenChannel
// completen de verdad contra un socket real, sin acoplar el test a la capa
// de red real de TastyTrade.
func startFakeDxLinkServer(t *testing.T) string {
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
				Type    string `json:"type"`
				Channel int    `json:"channel"`
			}
			if err := json.Unmarshal(msg, &env); err != nil {
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
			}
		}
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

// newConnectedDxLinkConn dialea y completa el handshake contra un
// startFakeDxLinkServer -- falla el test de inmediato si Connect() no
// termina en Connected()==true.
func newConnectedDxLinkConn(t *testing.T, wsURL string) *DxLinkConn {
	t.Helper()
	c := NewDxLinkConn(func() string { return wsURL }, func() string { return "test-token" })
	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("Connect failed against fake dxlink server: %v", err)
	}
	t.Cleanup(c.Close)
	return c
}
