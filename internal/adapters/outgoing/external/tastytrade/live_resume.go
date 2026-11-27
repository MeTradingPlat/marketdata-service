package tastytrade

import "time"

const resumeSettle = 2 * time.Minute

type resumeState struct {
	from        time.Time
	firstDataAt time.Time
}

func (p *CandlePool) holdResume(symbol string, from, _ time.Time) {
	if from.IsZero() {
		return
	}
	p.resumeMu.Lock()
	defer p.resumeMu.Unlock()
	if p.resume == nil {
		p.resume = make(map[string]resumeState)
	}
	cur, ok := p.resume[symbol]
	if !ok {
		p.resume[symbol] = resumeState{from: from}
		return
	}
	if cur.from.IsZero() || from.Before(cur.from) {
		cur.from = from
	}
	cur.firstDataAt = time.Time{}
	p.resume[symbol] = cur
}

func (p *CandlePool) noteResumeData(symbol string, now time.Time) {
	p.resumeMu.Lock()
	defer p.resumeMu.Unlock()
	cur, ok := p.resume[symbol]
	if !ok {
		return
	}
	if cur.firstDataAt.IsZero() {
		cur.firstDataAt = now
		p.resume[symbol] = cur
		return
	}
	if now.Sub(cur.firstDataAt) > resumeSettle {
		delete(p.resume, symbol)
	}
}
