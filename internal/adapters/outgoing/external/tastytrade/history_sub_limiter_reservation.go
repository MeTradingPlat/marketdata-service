package tastytrade

import "context"

type historySubReservation struct {
	limiter   *historySubLimiter
	remaining int
}

func (l *historySubLimiter) acquire(ctx context.Context, symbols int) (*historySubReservation, error) {
	if l == nil || symbols <= 0 {
		return nil, nil
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		l.mu.Lock()
		if res, ok := l.tryAcquireLocked(l.now(), symbols); ok {
			l.mu.Unlock()
			return res, nil
		}
		waitCh := l.waitChanLocked()
		timer, timerC := l.waitTimerLocked(l.now())
		l.mu.Unlock()
		if err := waitLimiterSignal(ctx, waitCh, timer, timerC); err != nil {
			return nil, err
		}
	}
}

func (r *historySubReservation) commitAdds(count int) {
	if r == nil || r.limiter == nil || count <= 0 {
		return
	}
	l := r.limiter
	l.mu.Lock()
	defer l.mu.Unlock()
	if r.remaining <= 0 {
		return
	}
	take := min(count, r.remaining)
	r.remaining -= take
	l.reserved = max(0, l.reserved-take)
	now := l.now()
	l.pruneLocked(now)
	l.recordLocked(now, take)
	l.notifyLocked()
}

func (r *historySubReservation) release() {
	if r == nil || r.limiter == nil {
		return
	}
	l := r.limiter
	l.mu.Lock()
	if r.remaining > 0 {
		l.reserved = max(0, l.reserved-r.remaining)
		r.remaining = 0
		l.notifyLocked()
	}
	l.mu.Unlock()
}
