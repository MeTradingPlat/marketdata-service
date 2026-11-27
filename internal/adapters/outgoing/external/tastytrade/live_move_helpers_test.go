package tastytrade

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type recordedSub struct {
	Channel  int
	Action   string
	Symbol   string
	FromTime int64
}

type recordedDxLinkSubs struct {
	mu   sync.Mutex
	subs []recordedSub
	ch   chan recordedSub
}

func (r *recordedDxLinkSubs) snapshot() []recordedSub {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedSub(nil), r.subs...)
}

func (r *recordedDxLinkSubs) waitCount(t *testing.T, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		select {
		case <-r.ch:
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for recorded sub %d/%d", i+1, n)
		}
	}
}

func startSubFeedRecordingServer(t *testing.T) (string, *recordedDxLinkSubs) {
	t.Helper()
	rec := &recordedDxLinkSubs{ch: make(chan recordedSub, 64)}
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		handleSubFeedConn(c, rec)
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http"), rec
}
