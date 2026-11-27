package tastytrade

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

func makeTestKeys(prefix string, n int) []string {
	keys := make([]string, n)
	for i := 0; i < n; i++ {
		keys[i] = fmt.Sprintf("%s-%d", prefix, i)
	}
	return keys
}

func TestChannelAllocator_ConcurrentBatches(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	a := newChannelAllocator(factory, nil, nil, 10)

	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for b := 0; b < 4; b++ {
		wg.Add(1)
		go func(batch int) {
			defer wg.Done()
			if _, err := a.allocate(context.Background(), makeTestKeys(fmt.Sprintf("B%d", batch), channelCapacity)...); err != nil {
				errs <- err
			}
		}(b)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("batch allocate: %v", err)
	}
	channels := a.allChannels()
	if len(channels) != 4 {
		t.Fatalf("expected 4 distinct channels, got %d", len(channels))
	}
	for _, ch := range channels {
		ch.mu.Lock()
		n := len(ch.symbols)
		ch.mu.Unlock()
		if n != channelCapacity {
			t.Fatalf("channel has %d keys, want %d", n, channelCapacity)
		}
	}
}
