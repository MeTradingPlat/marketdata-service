package tastytrade

func (p *CandlePool) CanSubscribeLive() bool {
	connected, total := p.allocator.stats()
	return connected > 0 || total == 0
}
