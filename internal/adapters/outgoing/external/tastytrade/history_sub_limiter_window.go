package tastytrade

import "time"

func (l *historySubLimiter) recordRemoves(count int) {
	if l == nil || count <= 0 {
		return
	}
	l.mu.Lock()
	now := l.now()
	l.pruneLocked(now)
	l.recordLocked(now, count)
	l.mu.Unlock()
}

func (l *historySubLimiter) pruneLocked(now time.Time) {
	cutoff := now.Add(-l.window)
	i := 0
	for i < len(l.events) && !l.events[i].at.After(cutoff) {
		l.used -= l.events[i].count
		i++
	}
	if i > 0 {
		l.events = l.events[i:]
	}
}

func (l *historySubLimiter) recordLocked(now time.Time, count int) {
	l.events = append(l.events, subChangeEvent{at: now, count: count})
	l.used += count
}
