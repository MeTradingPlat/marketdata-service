package tastytrade

import (
	"sync"
	"time"
)

const channelCapacity = 100

type pooledChannel struct {
	channel   *dxLinkChannel
	createdAt time.Time

	mu      sync.Mutex
	symbols map[string]struct{}
}

func newPooledChannel(ch *dxLinkChannel) *pooledChannel {
	return &pooledChannel{channel: ch, createdAt: time.Now(), symbols: make(map[string]struct{})}
}

func (p *pooledChannel) release(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.symbols, key)
}

func (p *pooledChannel) isRetired() bool {
	return p == nil || (p.channel != nil && p.channel.retired.Load())
}

func (p *pooledChannel) liveSymbols() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var symbols []string
	for key := range p.symbols {
		if symbol, ok := liveSymbolFromKey(key); ok {
			symbols = append(symbols, symbol)
		}
	}
	return symbols
}
