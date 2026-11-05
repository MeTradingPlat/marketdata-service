package tastytrade

import (
	"context"
	"testing"
	"time"
)

func TestDxLinkConn_CloseWakesReconnect(t *testing.T) {
	c := NewDxLinkConn(func() string { return "ws://127.0.0.1:1" }, func() string { return "" })
	c.sessionSaturated.Store(true)
	c.reconnectMu.Lock()
	c.reconnecting = true
	c.reconnectMu.Unlock()

	done := make(chan struct{})
	go func() {
		c.reconnectLoop(context.Background())
		close(done)
	}()

	c.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("expected reconnectLoop to exit within 2s of Close()")
	}
}
