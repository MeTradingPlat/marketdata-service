package tastytrade

import (
	"sync"
	"time"
)

const (
	defaultHistorySubChangesPerMinute = 8000
	historySubWindow                  = 60 * time.Second
)

type subChangeEvent struct {
	at    time.Time
	count int
}

type historySubLimiter struct {
	mu       sync.Mutex
	quota    int
	window   time.Duration
	now      func() time.Time
	events   []subChangeEvent
	used     int
	reserved int
	notifyCh chan struct{}
}

func newHistorySubLimiter(quota int) *historySubLimiter {
	return newHistorySubLimiterWithClock(quota, historySubWindow, time.Now)
}

func newHistorySubLimiterWithClock(quota int, window time.Duration, now func() time.Time) *historySubLimiter {
	if quota < 1 {
		quota = defaultHistorySubChangesPerMinute
	}
	return &historySubLimiter{quota: quota, window: window, now: now}
}

func (l *historySubLimiter) setQuota(quota int) {
	if l == nil {
		return
	}
	if quota < 1 {
		quota = defaultHistorySubChangesPerMinute
	}
	l.mu.Lock()
	l.quota = quota
	l.notifyLocked()
	l.mu.Unlock()
}
