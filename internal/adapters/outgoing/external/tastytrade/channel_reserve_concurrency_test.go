package tastytrade

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

func TestClaimLiveChannel_ConcurrentSingleKeyCapacity(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	p.allocator = newChannelAllocator(factory, p.wireChannel, p.handleConnectionReconnect, 10)

	const total = 250
	var wg sync.WaitGroup
	errs := make(chan error, total)
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			if _, err := p.claimLiveChannel(context.Background(), fmt.Sprintf("SYM%d", idx)); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("claimLiveChannel: %v", err)
	}

	seen := make(map[string]int, total)
	for _, ch := range p.allocator.allChannels() {
		ch.mu.Lock()
		n := len(ch.symbols)
		for k := range ch.symbols {
			seen[k]++
		}
		ch.mu.Unlock()
		if n > channelCapacity {
			t.Fatalf("channel has %d keys, max %d", n, channelCapacity)
		}
	}
	if len(seen) != total {
		t.Fatalf("expected %d unique keys, got %d", total, len(seen))
	}
	for k, count := range seen {
		if count != 1 {
			t.Fatalf("key %s appeared in %d channels", k, count)
		}
	}
}
