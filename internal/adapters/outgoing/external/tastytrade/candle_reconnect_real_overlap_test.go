package tastytrade

import (
	"context"
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestCandlePool_HandleConnectionReconnect_RealOverlapWithRetiredNewChannel(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := NewCandlePool(factory, 4)
	t.Cleanup(p.CloseAllConnections)

	ctx := context.Background()
	syms := make([]string, channelCapacity+1)
	for i := range syms {
		syms[i] = fmt.Sprintf("S%03d", i)
		if err := p.SubscribeLive(ctx, syms[i], time.Now().Add(-time.Minute), func(domain.Candle) {}, nil); err != nil {
			t.Fatalf("SubscribeLive %s: %v", syms[i], err)
		}
	}
	pc := p.allocator.connections[0]
	for _, ch := range pc.channels {
		ch.channel.retired.Store(true)
	}
	p.allocator.maxChannelsPerConn = 1

	var calls atomic.Int32
	blocked, release := make(chan struct{}), make(chan struct{})
	p.allocator.connFactory = func(ctx context.Context) (*DxLinkConn, error) {
		if calls.Add(1) == 1 {
			close(blocked)
			<-release
		}
		return factory(ctx)
	}

	done1 := make(chan struct{})
	go func() { defer close(done1); p.handleConnectionReconnect(ctx, pc) }()
	<-blocked
	for _, ch := range p.allocator.allChannels() {
		ch.channel.retired.Store(true)
	}
	done2 := make(chan struct{})
	go func() { defer close(done2); p.handleConnectionReconnect(ctx, pc) }()
	close(release)
	<-done1
	<-done2

	for _, sym := range syms {
		count := 0
		for _, ch := range p.allocator.allChannels() {
			if slices.Contains(ch.liveSymbols(), sym) {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("expected %s in exactly 1 live channel, got %d", sym, count)
		}
	}
}
