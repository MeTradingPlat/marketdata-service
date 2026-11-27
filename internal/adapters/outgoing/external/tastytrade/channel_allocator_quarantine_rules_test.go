package tastytrade

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestChannelAllocator_Quarantine_ContextAndSuccessRules(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	c := newConnectedDxLinkConn(t, wsURL)
	pc := newPooledConnection(c)
	a := newChannelAllocator(nil, nil, nil, 1)
	a.connections = []*pooledConnection{pc}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.openChannelOn(canceled, pc); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected Canceled, got %v", err)
	}
	if pc.consecutiveOpenTimeouts != 0 {
		t.Fatalf("canceled ctx should not count as timeout, got %d", pc.consecutiveOpenTimeouts)
	}

	now := time.Now()
	pc.recordOpenTimeout(now)
	pc.recordOpenTimeout(now)
	if _, err := a.openChannelOn(context.Background(), pc); err != nil {
		t.Fatalf("openChannelOn success failed: %v", err)
	}
	if pc.consecutiveOpenTimeouts != 0 || pc.quarantineStrikes != 0 || pc.isQuarantined(now) {
		t.Fatal("expected success to reset timeout count, strikes, and quarantine")
	}

	wantDurs := []time.Duration{2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 16 * time.Minute, 30 * time.Minute, 30 * time.Minute}
	for i, want := range wantDurs {
		if got := quarantineDuration(i + 1); got != want {
			t.Fatalf("strike %d: expected %v, got %v", i+1, want, got)
		}
	}
}

func TestChannelAllocator_Quarantine_FallbackAtCeiling(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	c := newConnectedDxLinkConn(t, wsURL)
	pc := newPooledConnection(c)
	pc.quarantinedUntil = time.Now().Add(10 * time.Minute)
	a := newChannelAllocator(nil, nil, nil, 1)
	a.connections = []*pooledConnection{pc}

	ch, err := a.allocate(context.Background())
	if err != nil {
		t.Fatalf("expected allocate to ignore quarantine at ceiling, got %v", err)
	}
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}
}
