package tastytrade

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestChannelAllocator_QuarantineSendsAlertWithFields(t *testing.T) {
	origTimeout := channelOpenTimeout
	channelOpenTimeout = 20 * time.Millisecond
	t.Cleanup(func() { channelOpenTimeout = origTimeout })

	var mu sync.Mutex
	var alertsSent []map[string]any
	origNotify := notifyQuarantineAlert
	notifyQuarantineAlert = func(level, msg string, meta map[string]any) {
		mu.Lock()
		defer mu.Unlock()
		meta["__level"] = level
		alertsSent = append(alertsSent, meta)
	}
	t.Cleanup(func() { notifyQuarantineAlert = origNotify })

	var connNum atomic.Int32
	wsURL, _ := startSelectiveAckDxLinkServer(t, &connNum)
	a := newChannelAllocator(func(ctx context.Context) (*DxLinkConn, error) {
		c := NewDxLinkConn(func() string { return wsURL }, func() string { return "tok" })
		return c, c.Connect(ctx)
	}, nil, nil, 1)
	t.Cleanup(func() {
		for _, pc := range a.drainAll() {
			pc.conn.Close()
		}
	})

	for i := 0; i < 3; i++ {
		if _, err := a.allocate(context.Background()); !errors.Is(err, errChannelOpenTimeout) {
			t.Fatalf("attempt %d: got %v", i+1, err)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(alertsSent) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alertsSent))
	}
	m := alertsSent[0]
	for _, key := range []string{"connID", "strikes", "duration", "force_reconnect", "open_channels", "last_message_age", "open_sessions", "last_timeout_cause", "socket_stats"} {
		if _, ok := m[key]; !ok {
			t.Fatalf("missing alert metadata key %q", key)
		}
	}
	if m["__level"] != "WARN" {
		t.Fatalf("expected WARN level, got %v", m["__level"])
	}
}
