package tastytrade

func (p *pooledConnection) openChannelCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	active := 0
	for _, ch := range p.channels {
		if !ch.isRetired() {
			active++
		}
	}
	return active
}
