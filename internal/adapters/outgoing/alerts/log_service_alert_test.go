package alerts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestSend(t *testing.T) {
	upgrader := websocket.Upgrader{}
	ch := make(chan []byte, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Gateway-Passed") != "true" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		_, msg, err := c.ReadMessage()
		if err == nil {
			ch <- msg
		}
	}))
	defer server.Close()

	t.Setenv("LOG_SERVICE_WS_URL", "ws"+strings.TrimPrefix(server.URL, "http"))

	err := Send(context.Background(), "WARN", "prueba", map[string]any{"k": "v"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	raw := <-ch
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	for _, key := range []string{"nivel", "mensaje", "servicioOrigen", "categoria"} {
		if got[key] == nil || got[key] == "" {
			t.Errorf("missing or empty %s", key)
		}
	}
	if _, ok := got["metadatos"].(string); !ok {
		t.Errorf("expected metadatos to be string, got %T", got["metadatos"])
	}
}
