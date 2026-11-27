package tastytrade

import (
	"context"
	"testing"
	"time"
)

func TestDxLinkConn_CloseCancelsBlockedOnReconnect(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	c := NewDxLinkConn(func() string { return wsURL }, func() string { return "tok" })

	started := make(chan struct{})
	done := make(chan struct{})
	c.OnReconnect(func(ctx context.Context, _ *DxLinkConn) {
		close(started)
		<-ctx.Done()
		close(done)
	})

	if err := c.performReconnect(context.Background()); err != nil {
		t.Fatalf("performReconnect: %v", err)
	}
	<-started
	c.Close()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("expected Close() to cancel blocked onReconnect")
	}
}
