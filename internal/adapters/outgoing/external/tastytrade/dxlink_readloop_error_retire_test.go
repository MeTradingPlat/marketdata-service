package tastytrade

import (
	"context"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestDxLinkConn_HandleError_RetiresChannelsAndOrphansPoolSymbols(t *testing.T) {
	for _, errType := range []string{"UNAUTHORIZED", "INVALID_MESSAGE"} {
		t.Run(errType, func(t *testing.T) {
			wsURL := startFakeDxLinkServer(t)
			factory, _ := newTestConnFactory(t, wsURL)
			p := NewCandlePool(factory, 1)
			t.Cleanup(p.CloseAllConnections)

			ctx := context.Background()
			if err := p.SubscribeLive(ctx, "AAPL", time.Now().Add(-time.Minute), func(domain.Candle) {}, nil); err != nil {
				t.Fatalf("SubscribeLive: %v", err)
			}
			ch := p.liveChannelFor("AAPL").channel
			conn := p.allocator.connections[0].conn
			conn.closing.Store(true)

			conn.handleError(ctx, inboundEnvelope{Type: "ERROR", Error: errType, Message: "test"})

			if !ch.retired.Load() {
				t.Fatalf("expected channel retired after %s", errType)
			}
			orphans := p.orphanedLiveSymbols()
			if len(orphans) != 1 || orphans[0] != "AAPL" {
				t.Fatalf("expected AAPL orphan after %s, got %v", errType, orphans)
			}
		})
	}
}
