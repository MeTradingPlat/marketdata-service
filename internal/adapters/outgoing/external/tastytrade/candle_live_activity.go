package tastytrade

import "time"

func (p *CandlePool) ensureActivityMaps() {
	if p.lastEventAt == nil {
		p.lastEventAt = make(map[string]time.Time)
	}
	if p.pendingRevive == nil {
		p.pendingRevive = make(map[string]reviveState)
	}
	if p.orphanSince == nil {
		p.orphanSince = make(map[string]time.Time)
	}
	if p.lastRoutedAt == nil {
		p.lastRoutedAt = make(map[string]time.Time)
	}
	if p.lastTradeAt == nil {
		p.lastTradeAt = make(map[string]time.Time)
	}
	if p.lastNewBarAt == nil {
		p.lastNewBarAt = make(map[string]time.Time)
	}
	if p.lastBarTS == nil {
		p.lastBarTS = make(map[string]time.Time)
	}
	if p.subscribedAt == nil {
		p.subscribedAt = make(map[string]time.Time)
	}
	if p.silentHealHistory == nil {
		p.silentHealHistory = make(map[string][]time.Time)
	}
}

func (p *CandlePool) noteLiveEvent(symbol string, now time.Time) {
	p.activityMu.Lock()
	p.ensureActivityMaps()
	p.lastEventAt[symbol] = now
	p.activityMu.Unlock()
}

func (p *CandlePool) noteRouted(symbol string, now time.Time) {
	p.activityMu.Lock()
	p.ensureActivityMaps()
	p.lastRoutedAt[symbol] = now
	p.activityMu.Unlock()
}

func (p *CandlePool) noteTrade(symbol string, now time.Time) {
	p.activityMu.Lock()
	p.ensureActivityMaps()
	p.lastTradeAt[symbol] = now
	p.activityMu.Unlock()
}
