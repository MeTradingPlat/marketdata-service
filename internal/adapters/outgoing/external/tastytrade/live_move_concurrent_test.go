package tastytrade

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestMoveLiveSymbolChannel_ConcurrentSameSymbol(t *testing.T) {
	wsURL, rec := startSubFeedRecordingServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.ensureActivityMaps()
	p.allocator = newChannelAllocator(factory, p.wireChannel, p.handleConnectionReconnect, 4)
	p.SetMaxChannelsPerConn(1)

	ctx := context.Background()
	now := time.Date(2026, 11, 26, 15, 0, 0, 0, time.UTC)
	if err := p.SubscribeLive(ctx, "NVDA", now, func(domain.Candle) {}, nil); err != nil {
		t.Fatalf("SubscribeLive: %v", err)
	}
	rec.waitCount(t, 3)
	oldCh := p.liveChannelFor("NVDA")

	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := p.moveLiveSymbolChannel(ctx, "NVDA", oldCh, now, 0, now)
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil && !errors.Is(err, errMoveInProgress) {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	key := candleKey("NVDA", domain.M1)
	owners := 0
	for _, ch := range p.allocator.allChannels() {
		if channelHasKey(ch, key) {
			owners++
		}
	}
	if owners != 1 {
		t.Fatalf("expected NVDA reserved on exactly 1 channel, got %d", owners)
	}
}
