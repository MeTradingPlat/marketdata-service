package tastytrade

import (
	"sync"
	"time"
)

const (
	retiredDispatchTTL = 30 * time.Second
	retiredDispatchCap = 50000
)

type retiredDispatch struct {
	mu        sync.Mutex
	retiredAt map[string]time.Time
}

func (r *retiredDispatch) mark(key string, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.retiredAt == nil {
		r.retiredAt = make(map[string]time.Time)
	}
	if len(r.retiredAt) >= retiredDispatchCap {
		r.purgeExpired(now)
	}
	if len(r.retiredAt) >= retiredDispatchCap {
		return
	}
	r.retiredAt[key] = now
}

func (r *retiredDispatch) wasRecentlyRetired(key string, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	at, ok := r.retiredAt[key]
	return ok && now.Sub(at) <= retiredDispatchTTL
}

func (r *retiredDispatch) purgeExpired(now time.Time) {
	for key, at := range r.retiredAt {
		if now.Sub(at) > retiredDispatchTTL {
			delete(r.retiredAt, key)
		}
	}
}
