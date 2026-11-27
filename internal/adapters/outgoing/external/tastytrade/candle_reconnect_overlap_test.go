package tastytrade

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestCandlePool_HandleConnectionReconnect_OverlappingKeepsLiveChannels(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := NewCandlePool(factory, 4)
	t.Cleanup(p.CloseAllConnections)

	ctx := context.Background()
	for _, sym := range []string{"AAA", "BBB"} {
		if err := p.SubscribeLive(ctx, sym, time.Now().Add(-time.Minute), func(domain.Candle) {}, nil); err != nil {
			t.Fatalf("SubscribeLive %s: %v", sym, err)
		}
	}
	pc := p.allocator.connections[0]
	for _, ch := range pc.channels {
		ch.channel.retired.Store(true)
	}

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.handleConnectionReconnect(ctx, pc)
		}()
	}
	wg.Wait()

	for _, sym := range []string{"AAA", "BBB"} {
		count := 0
		for _, ch := range p.allocator.allChannels() {
			for _, s := range ch.liveSymbols() {
				if s == sym {
					count++
				}
			}
		}
		if count != 1 {
			t.Fatalf("expected %s in exactly 1 live channel, got %d", sym, count)
		}
	}
}
