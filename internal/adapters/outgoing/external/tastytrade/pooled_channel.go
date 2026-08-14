package tastytrade

import "sync"

const channelCapacity = 100

type pooledChannel struct {
	channel *dxLinkChannel

	mu      sync.Mutex
	symbols map[string]struct{}
}

func newPooledChannel(ch *dxLinkChannel) *pooledChannel {
	return &pooledChannel{channel: ch, symbols: make(map[string]struct{})}
}

func (p *pooledChannel) hasRoom() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.symbols) < channelCapacity
}

func (p *pooledChannel) occupy(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.symbols[key] = struct{}{}
}

func (p *pooledChannel) release(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.symbols, key)
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
