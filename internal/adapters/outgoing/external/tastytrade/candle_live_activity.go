package tastytrade

import "time"

func (p *CandlePool) ensureActivityMaps() {
	for _, m := range []*map[string]time.Time{
		&p.lastEventAt, &p.orphanSince, &p.lastRoutedAt,
		&p.lastTradeAt, &p.lastNewBarAt, &p.lastBarTS, &p.subscribedAt,
	} {
		if *m == nil {
			*m = make(map[string]time.Time)
		}
	}
	if p.pendingRevive == nil {
		p.pendingRevive = make(map[string]reviveState)
	}
	if p.silentHealHistory == nil {
		p.silentHealHistory = make(map[string][]time.Time)
	}
}

func (p *CandlePool) noteActivity(target func() map[string]time.Time, symbol string, now time.Time) {
	p.activityMu.Lock()
	p.ensureActivityMaps()
	target()[symbol] = now
	p.activityMu.Unlock()
}

func (p *CandlePool) noteLiveEvent(symbol string, now time.Time) {
	p.noteActivity(func() map[string]time.Time { return p.lastEventAt }, symbol, now)
}

func (p *CandlePool) noteRouted(symbol string, now time.Time) {
	p.noteActivity(func() map[string]time.Time { return p.lastRoutedAt }, symbol, now)
}

func (p *CandlePool) noteTrade(symbol string, now time.Time) {
	p.noteActivity(func() map[string]time.Time { return p.lastTradeAt }, symbol, now)
}
