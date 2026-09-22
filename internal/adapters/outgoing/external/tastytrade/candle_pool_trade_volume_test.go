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

// startFakeDxLinkServerWithTrade es startFakeDxLinkServer (ver
// dxlink_fake_server_test.go) mas el envio de un FEED_DATA "Trade" apenas
// llega la FEED_SUBSCRIPTION -- el snapshot en vivo que confirmamos el
// 2026-09-22 contra el pool real (dayVolume llega apenas se suscribe, sin
// esperar un tick nuevo).
func startFakeDxLinkServerWithTrade(t *testing.T, dayVolumes map[string]float64) string {
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
				Type    string                 `json:"type"`
				Channel int                    `json:"channel"`
				Add     []feedSubscriptionItem `json:"add"`
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
			case "FEED_SUBSCRIPTION":
				if len(env.Add) == 0 || env.Add[0].Type != "Trade" {
					continue
				}
				var data []interface{}
				for _, item := range env.Add {
					v, ok := dayVolumes[item.Symbol]
					if !ok {
						continue
					}
					data = append(data, "Trade", []interface{}{item.Symbol, v})
				}
				_ = conn.WriteJSON(map[string]any{"type": "FEED_DATA", "channel": env.Channel, "data": data})
			}
		}
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

func TestFetchDayVolumes_ReturnsTheDayVolumeOfEachSymbolThatHasOne(t *testing.T) {
	wsURL := startFakeDxLinkServerWithTrade(t, map[string]float64{"SPY": 5_468_232, "AAPL": 9_491_383})
	connFactory := func(ctx context.Context) (*DxLinkConn, error) {
		c := NewDxLinkConn(func() string { return wsURL }, func() string { return "token" })
		if err := c.Connect(ctx); err != nil {
			return nil, err
		}
		return c, nil
	}
	pool := NewCandlePool(connFactory, defaultMaxConnections)

	got := pool.FetchDayVolumes(context.Background(), []string{"SPY", "AAPL", "MDXH"})

	if got["SPY"] != 5_468_232 {
		t.Errorf("SPY = %d, want 5468232", got["SPY"])
	}
	if got["AAPL"] != 9_491_383 {
		t.Errorf("AAPL = %d, want 9491383", got["AAPL"])
	}
	if _, ok := got["MDXH"]; ok {
		t.Errorf("MDXH should be absent (the fake server has no dayVolume for it), got %v", got["MDXH"])
	}
}

func TestFetchDayVolumes_EmptyInputReturnsEmptyWithoutDialing(t *testing.T) {
	dialed := false
	connFactory := func(ctx context.Context) (*DxLinkConn, error) {
		dialed = true
		return nil, nil
	}
	pool := NewCandlePool(connFactory, defaultMaxConnections)

	got := pool.FetchDayVolumes(context.Background(), nil)

	if len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
	if dialed {
		t.Fatal("should not open any connection for an empty symbol list")
	}
}
