package tastytrade

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestFetchHistoryBatch_RemovesBeforeRelease(t *testing.T) {
	wsURL, rec := startSubFeedRecordingServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.allocator = newChannelAllocator(factory, p.wireChannel, p.handleConnectionReconnect, 1)

	ch, err := p.allocator.allocate(context.Background())
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := p.FetchHistoryBatch(ctx, domain.D1, map[string]time.Time{"AAPL": time.Now().Add(-time.Hour)})
		done <- err
	}()
	rec.waitCount(t, 1)

	ch.mu.Lock()
	cancel()
	rec.waitCount(t, 2)
	ch.mu.Unlock()

	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if channelHasKey(ch, candleKey("AAPL", domain.D1)) {
		t.Fatal("expected key released after batch cleanup")
	}
}
