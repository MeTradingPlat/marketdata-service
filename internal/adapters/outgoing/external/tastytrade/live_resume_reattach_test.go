package tastytrade

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestCandlePool_Resume_ReattachOrphansUsesFormingTimestamp(t *testing.T) {
	wsURL, subCh := startSubRecordingDxLinkServer(t)
	var failDial atomic.Bool
	p := NewCandlePool(func(ctx context.Context) (*DxLinkConn, error) {
		if failDial.Load() {
			return nil, errors.New("dial blocked")
		}
		c := NewDxLinkConn(func() string { return wsURL }, func() string { return "tok" })
		return c, c.Connect(ctx)
	}, 2)
	t.Cleanup(p.CloseAllConnections)

	ctx := context.Background()
	if err := p.SubscribeLive(ctx, "AAPL", time.Now().Add(-time.Minute), func(domain.Candle) {}, nil); err != nil {
		t.Fatalf("SubscribeLive: %v", err)
	}
	<-subCh

	formingTS := time.Now().UTC().Truncate(time.Minute).Add(-7 * time.Minute)
	p.currentMu.Lock()
	p.current["AAPL"] = domain.Candle{Symbol: "AAPL", Timestamp: formingTS}
	p.currentMu.Unlock()

	pc := p.allocator.connections[0]
	pc.conn.cleanup()
	p.allocator.drainAll()
	failDial.Store(true)
	cancelCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	p.handleConnectionReconnect(cancelCtx, pc)

	failDial.Store(false)
	t1 := time.Now()
	p.ReattachOrphans(ctx, t1, 10)
	if n := p.ReattachOrphans(ctx, t1.Add(orphanReattachGrace+time.Second), 10); n != 1 {
		t.Fatalf("expected 1 reattached orphan, got %d", n)
	}
	gotFrom := <-subCh
	if gotFrom != formingTS.UnixMilli() {
		t.Fatalf("expected fromTime %d, got %d", formingTS.UnixMilli(), gotFrom)
	}
}
