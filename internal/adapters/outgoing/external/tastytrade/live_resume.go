package tastytrade

import "time"

const resumeSettle = 2 * time.Minute

type resumeState struct {
	from           time.Time
	resubscribedAt time.Time
	firstDataAt    time.Time
	lastReplayAt   time.Time
}

func (p *CandlePool) holdResume(symbol string, from, now time.Time) {
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
		p.resume[symbol] = resumeState{from: from, resubscribedAt: now}
		return
	}
	if cur.from.IsZero() || from.Before(cur.from) {
		cur.from = from
	}
	cur.resubscribedAt = now
	cur.firstDataAt = time.Time{}
	cur.lastReplayAt = time.Time{}
	p.resume[symbol] = cur
}

func (p *CandlePool) noteResumeData(symbol string, candleTS, now time.Time) {
	p.resumeMu.Lock()
	defer p.resumeMu.Unlock()
	cur, ok := p.resume[symbol]
	if !ok {
		return
	}
	isReplay := !candleTS.IsZero() && !cur.resubscribedAt.IsZero() && candleTS.Before(cur.resubscribedAt)
	if cur.firstDataAt.IsZero() {
		cur.firstDataAt = now
	}
	if isReplay {
		cur.lastReplayAt = now
		p.resume[symbol] = cur
		return
	}
	p.resume[symbol] = cur
	if now.Sub(resumeAnchor(cur)) > resumeSettle {
		delete(p.resume, symbol)
	}
}

func resumeAnchor(cur resumeState) time.Time {
	if cur.lastReplayAt.After(cur.firstDataAt) {
		return cur.lastReplayAt
	}
	return cur.firstDataAt
}
