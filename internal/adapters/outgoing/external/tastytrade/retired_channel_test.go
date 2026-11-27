package tastytrade

import (
	"context"
	"testing"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestPooledChannel_RetiredExcludedFromPoolAndTakeRetiredSymbols(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	a := newChannelAllocator(factory, nil, nil, 2)
	ctx := context.Background()
	ch1, err := a.allocate(ctx)
	if err != nil {
		t.Fatalf("allocate ch1: %v", err)
	}
	ch1.occupy(candleKey("AAA", domain.M1))
	for i := 0; i < channelCapacity; i++ {
		ch1.occupy(candleKey("FILL"+string(rune('A'+i)), domain.M1))
	}
	ch2, err := a.allocate(ctx)
	if err != nil {
		t.Fatalf("allocate ch2: %v", err)
	}
	ch2.occupy(candleKey("BBB", domain.M1))

	ch1.channel.retired.Store(true)
	pc := a.connections[0]
	if pc.owns(ch1) || !pc.owns(ch2) {
		t.Fatalf("owns mismatch: ch1=%v ch2=%v", pc.owns(ch1), pc.owns(ch2))
	}
	if !pc.hasRoomForNewChannel(2) {
		t.Fatal("expected hasRoomForNewChannel true when one channel is retired")
	}
	if got := a.allChannels(); len(got) != 1 || got[0] != ch2 {
		t.Fatalf("expected allChannels=[ch2], got %d", len(got))
	}
	syms := pc.takeRetiredSymbols()
	if len(syms) == 0 || !pc.owns(ch2) {
		t.Fatalf("expected retired symbols and ch2 kept, got %v owns(ch2)=%v", syms, pc.owns(ch2))
	}
}

func TestDxLinkConn_CleanupRetiresChannels(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	c := newConnectedDxLinkConn(t, wsURL)
	ch, err := c.OpenChannel(context.Background())
	if err != nil {
		t.Fatalf("OpenChannel: %v", err)
	}
	c.cleanup()
	if !ch.retired.Load() {
		t.Fatal("expected cleanup to mark channel retired")
	}
}
