package tastytrade

import "time"

func (p *CandlePool) noteNewBar(symbol string, barTS, now time.Time) {
	if barTS.IsZero() {
		return
	}
	p.activityMu.Lock()
	p.ensureActivityMaps()
	if barTS.After(p.lastBarTS[symbol]) {
		p.lastBarTS[symbol] = barTS
		p.lastNewBarAt[symbol] = now
	}
	p.activityMu.Unlock()
}

func (p *CandlePool) noteSubscribed(symbol string, now time.Time) {
	p.noteActivity(func() map[string]time.Time { return p.subscribedAt }, symbol, now)
}

func (p *CandlePool) resetActivityMaps() {
	p.activityMu.Lock()
	p.lastEventAt = make(map[string]time.Time)
	p.pendingRevive = make(map[string]reviveState)
	p.lastRoutedAt = make(map[string]time.Time)
	p.lastTradeAt = make(map[string]time.Time)
	p.lastNewBarAt = make(map[string]time.Time)
	p.lastBarTS = make(map[string]time.Time)
	p.subscribedAt = make(map[string]time.Time)
	p.silentHealHistory = make(map[string][]time.Time)
	p.activityMu.Unlock()
	p.clearResume()
}
