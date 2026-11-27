package tastytrade

import "time"

func (p *CandlePool) knownBarTime(symbol string) time.Time {
	p.currentMu.Lock()
	defer p.currentMu.Unlock()
	var best time.Time
	if cur, ok := p.current[symbol]; ok && !cur.Timestamp.IsZero() {
		best = cur.Timestamp
	}
	if last, ok := p.lastClosed[symbol]; ok && !last.Timestamp.IsZero() && (best.IsZero() || last.Timestamp.Before(best)) {
		best = last.Timestamp
	}
	return best
}

func (p *CandlePool) resolveResume(symbol string, from, now time.Time) time.Time {
	p.resumeMu.Lock()
	cur, ok := p.resume[symbol]
	var held time.Time
	if ok {
		held = cur.from
	}
	p.resumeMu.Unlock()

	res := from
	if !held.IsZero() && (res.IsZero() || held.Before(res)) {
		res = held
	}
	if res.IsZero() {
		res = p.knownBarTime(symbol)
	}
	if res.IsZero() {
		res = now.Add(-5 * time.Minute)
	}
	p.holdResume(symbol, res, now)
	return res
}

func (p *CandlePool) clearResume() {
	p.resumeMu.Lock()
	p.resume = make(map[string]resumeState)
	p.resumeMu.Unlock()
}
