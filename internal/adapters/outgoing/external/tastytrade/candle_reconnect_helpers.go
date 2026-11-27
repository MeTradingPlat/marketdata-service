package tastytrade

import "time"

func (p *CandlePool) unattachedLiveSymbols(symbols []string) []string {
	var pending []string
	for _, symbol := range symbols {
		if p.liveChannelFor(symbol) == nil {
			pending = append(pending, symbol)
		}
	}
	return pending
}

func (p *CandlePool) markPendingRevive(symbols []string, now time.Time) {
	p.activityMu.Lock()
	p.ensureActivityMaps()
	for _, symbol := range symbols {
		if last, ok := p.lastEventAt[symbol]; ok && now.Sub(last) <= 10*time.Minute {
			p.pendingRevive[symbol] = reviveState{reconnectedAt: now}
		}
	}
	p.activityMu.Unlock()
}

func (p *CandlePool) clearPendingRevive(symbols []string) {
	p.activityMu.Lock()
	for _, symbol := range symbols {
		delete(p.pendingRevive, symbol)
	}
	p.activityMu.Unlock()
}
