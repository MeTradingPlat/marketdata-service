package tastytrade

import (
	"context"
	"testing"
)

func TestChannelAllocator_PartialFitAndOverCapacity(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	a := newChannelAllocator(factory, nil, nil, 10)

	if _, err := a.allocate(context.Background(), makeTestKeys("OVER", channelCapacity+1)...); err == nil {
		t.Fatal("expected error for > channelCapacity keys")
	}
	if a.connectionCount() != 0 {
		t.Fatalf("expected 0 connections after over-capacity error, got %d", a.connectionCount())
	}

	ch1, err := a.allocate(context.Background(), makeTestKeys("A", 37)...)
	if err != nil {
		t.Fatalf("allocate 37: %v", err)
	}
	ch1.mu.Lock()
	reserved := len(ch1.symbols)
	ch1.mu.Unlock()
	if reserved != 37 {
		t.Fatalf("expected new channel to start with 37 reserved keys, got %d", reserved)
	}

	ch2, err := a.allocate(context.Background(), makeTestKeys("B", 100)...)
	if err != nil {
		t.Fatalf("allocate 100: %v", err)
	}
	if ch1 == ch2 {
		t.Fatal("expected 100-key batch to get a different channel")
	}

	ch3, err := a.allocate(context.Background(), makeTestKeys("C", 63)...)
	if err != nil {
		t.Fatalf("allocate 63: %v", err)
	}
	if ch3 != ch1 {
		t.Fatal("expected 63-key batch to fit in first channel")
	}
}
