package tastytrade

import (
	"context"
	"time"
)

func (l *historySubLimiter) tryAcquireLocked(now time.Time, symbols int) (*historySubReservation, bool) {
	l.pruneLocked(now)
	need := 3 * symbols
	if (l.used != 0 || l.reserved != 0) && l.used+l.reserved+need > l.quota {
		return nil, false
	}
	l.reserved += need
	return &historySubReservation{limiter: l, remaining: need}, true
}

func (l *historySubLimiter) waitTimerLocked(now time.Time) (*time.Timer, <-chan time.Time) {
	if len(l.events) == 0 {
		return nil, nil
	}
	wait := l.events[0].at.Add(l.window).Sub(now)
	if wait < time.Millisecond {
		wait = time.Millisecond
	}
	t := time.NewTimer(wait)
	return t, t.C
}

func waitLimiterSignal(ctx context.Context, waitCh <-chan struct{}, timer *time.Timer, timerC <-chan time.Time) error {
	if timer != nil {
		defer timer.Stop()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-waitCh:
		return nil
	case <-timerC:
		return nil
	}
}

func (l *historySubLimiter) waitChanLocked() chan struct{} {
	if l.notifyCh == nil {
		l.notifyCh = make(chan struct{})
	}
	return l.notifyCh
}

func (l *historySubLimiter) notifyLocked() {
	if l.notifyCh != nil {
		close(l.notifyCh)
		l.notifyCh = nil
	}
}
