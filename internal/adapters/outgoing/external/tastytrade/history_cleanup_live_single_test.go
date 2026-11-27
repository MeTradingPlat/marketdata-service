package tastytrade

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestFetchHistory_CleanupSkipsLiveOnSameChannel(t *testing.T) {
	wsURL, rec := startSubFeedRecordingServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.allocator = newChannelAllocator(factory, p.wireChannel, p.handleConnectionReconnect, 2)

	ch, err := p.allocator.allocate(context.Background())
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := p.FetchHistory(ctx, "AAPL", domain.M1, time.Now().Add(-time.Hour))
		done <- err
	}()
	rec.waitCount(t, 1)
	p.registerLiveSub("AAPL", func(domain.Candle) {}, nil, false, 0)
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	for _, s := range rec.snapshot() {
		if s.Action == "remove" {
			t.Fatalf("unexpected remove when live on same channel: %+v", rec.snapshot())
		}
	}
	if !channelHasKey(ch, candleKey("AAPL", domain.M1)) {
		t.Fatal("expected AAPL M1 key preserved on channel")
	}
}

func TestHostsLiveSubscription_SkipsAllocatorWhenNotLive(t *testing.T) {
	p := newTestPool()
	if p.hostsLiveSubscription(nil, "MSFT", domain.M1) {
		t.Fatal("expected false when symbol is not live")
	}
	p.registerLiveSub("AAPL", func(domain.Candle) {}, nil, false, 0)
	if p.hostsLiveSubscription(nil, "AAPL", domain.D1) {
		t.Fatal("expected false for non-M1 timeframe")
	}
}
