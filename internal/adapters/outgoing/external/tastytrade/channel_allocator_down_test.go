package tastytrade

import (
	"context"
	"testing"
)

func TestChannelAllocator_SkipsDisconnectedConnection(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	a := newChannelAllocator(factory, nil, nil, 2)
	t.Cleanup(func() {
		for _, pc := range a.drainAll() {
			pc.conn.Close()
		}
	})
	ctx := context.Background()
	if _, err := a.allocate(ctx); err != nil {
		t.Fatalf("allocate first: %v", err)
	}
	down := a.connections[0]
	down.conn.cleanup()

	ch, err := a.allocate(ctx)
	if err != nil {
		t.Fatalf("expected allocation on another connection, got %v", err)
	}
	if len(a.connections) != 2 || down.owns(ch) || !a.connections[1].owns(ch) {
		t.Fatalf("expected channel on a new connection, connections=%d", len(a.connections))
	}
}
