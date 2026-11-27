package tastytrade

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestDxLinkConn_ReconnectsAfterCreatorCtxCanceled(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	c := NewDxLinkConn(func() string { return wsURL }, func() string { return "tok" })
	t.Cleanup(c.Close)

	reconnected := make(chan struct{}, 1)
	c.OnReconnect(func(context.Context, *DxLinkConn) {
		select {
		case reconnected <- struct{}{}:
		default:
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	cancel()

	c.mu.RLock()
	_ = c.conn.UnderlyingConn().Close()
	c.mu.RUnlock()

	select {
	case <-reconnected:
	case <-time.After(8 * time.Second):
		t.Fatal("expected connection to reconnect even after creator ctx was canceled")
	}
	if !c.Connected() {
		t.Fatal("expected Connected() true after reconnect")
	}
}

func TestDxLinkConn_RepeatedConnectsDoNotMultiplyWatchLoops(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	c := NewDxLinkConn(func() string { return wsURL }, func() string { return "tok" })
	t.Cleanup(c.Close)

	if err := c.Connect(context.Background()); err != nil {
		t.Fatalf("initial Connect: %v", err)
	}
	base := runtime.NumGoroutine()
	for i := 0; i < 6; i++ {
		c.cleanup()
		if err := c.Connect(context.Background()); err != nil {
			t.Fatalf("Connect %d: %v", i, err)
		}
	}
	runtime.Gosched()
	if got := runtime.NumGoroutine(); got > base+3 {
		t.Fatalf("goroutines grew across reconnects: base=%d got=%d", base, got)
	}
}
