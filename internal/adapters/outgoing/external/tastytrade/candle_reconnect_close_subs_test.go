package tastytrade

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestCandlePool_Resubscribe_CloseAllConnectionsLeavesNoLiveSubs(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	inFlight := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	p := NewCandlePool(func(ctx context.Context) (*DxLinkConn, error) {
		if calls.Add(1) == 1 {
			close(inFlight)
			<-release
		}
		c := NewDxLinkConn(func() string { return wsURL }, func() string { return "tok" })
		return c, c.Connect(ctx)
	}, 2)

	ctx := context.Background()
	p.liveMu.Lock()
	p.liveSubs["AAA"] = func(domain.Candle) {}
	p.liveMu.Unlock()
	p.allocator.connections = append(p.allocator.connections, newPooledConnection(newConnectedDxLinkConn(t, wsURL)))
	p.allocator.connections[0].conn.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		p.resubscribeReconnectSymbols(ctx, []string{"AAA"}, map[string]time.Time{"AAA": time.Now()})
	}()

	select {
	case <-inFlight:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for in-flight SubscribeLive")
	}
	p.CloseAllConnections()
	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for resubscribeReconnectSymbols")
	}

	if p.LiveSubscribed("AAA") {
		t.Fatal("expected AAA not re-registered in liveSubs after CloseAllConnections")
	}
}
