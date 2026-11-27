package tastytrade

func (p *pooledChannel) occupy(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.symbols[key] = struct{}{}
}
