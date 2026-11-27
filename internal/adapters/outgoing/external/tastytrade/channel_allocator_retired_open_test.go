package tastytrade

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestClaimLiveChannel_RetiredOnOpenRetriesAndSucceeds(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	var opened atomic.Int32
	var retiredCh *dxLinkChannel
	p.allocator = newChannelAllocator(factory, func(ch *dxLinkChannel) {
		p.wireChannel(ch)
		if opened.Add(1) == 1 {
			retiredCh = ch
			ch.retired.Store(true)
		}
	}, p.handleConnectionReconnect, 2)

	ch, err := p.claimLiveChannel(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("claimLiveChannel: %v", err)
	}
	if !p.allocator.owns(ch) || !channelHasKey(ch, candleKey("AAPL", domain.M1)) {
		t.Fatal("expected owned channel with AAPL M1 reserved")
	}
	for _, c := range p.allocator.allChannels() {
		if c.channel == retiredCh {
			t.Fatal("retired channel must not appear in allChannels")
		}
	}
}
