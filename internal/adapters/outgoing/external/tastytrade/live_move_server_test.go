package tastytrade

import (
	"encoding/json"

	"github.com/gorilla/websocket"
)

func handleSubFeedConn(c *websocket.Conn, rec *recordedDxLinkSubs) {
	for {
		_, data, err := c.ReadMessage()
		if err != nil {
			return
		}
		var m struct {
			Type    string `json:"type"`
			Channel int    `json:"channel"`
			Add     []struct {
				Symbol   string `json:"symbol"`
				FromTime int64  `json:"fromTime"`
			} `json:"add"`
			Remove []struct {
				Symbol string `json:"symbol"`
			} `json:"remove"`
		}
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		switch m.Type {
		case "SETUP":
			_ = c.WriteJSON(map[string]any{"type": "SETUP", "channel": 0, "version": "0.1"})
		case "AUTH":
			_ = c.WriteJSON(map[string]any{"type": "AUTH_STATE", "channel": 0, "state": "AUTHORIZED"})
		case "CHANNEL_REQUEST":
			_ = c.WriteJSON(map[string]any{"type": "CHANNEL_OPENED", "channel": m.Channel, "service": "FEED"})
		case "FEED_SETUP":
			_ = c.WriteJSON(map[string]any{"type": "FEED_CONFIG", "channel": m.Channel})
		case "FEED_SUBSCRIPTION":
			rec.mu.Lock()
			for _, a := range m.Add {
				s := recordedSub{m.Channel, "add", a.Symbol, a.FromTime}
				rec.subs = append(rec.subs, s)
				rec.ch <- s
			}
			for _, rm := range m.Remove {
				s := recordedSub{m.Channel, "remove", rm.Symbol, 0}
				rec.subs = append(rec.subs, s)
				rec.ch <- s
			}
			rec.mu.Unlock()
		}
	}
}
