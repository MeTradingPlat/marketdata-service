package main

import (
	"math/rand"
	"sync"
	"time"
)

const (
	reconcileBackoffBase   = 5 * time.Minute
	reconcileBackoffMax    = time.Hour
	reconcileBackoffJitter = 0.2
	reconcileBackoffCapAt  = 5
)

type backoffEntry struct {
	failures    int
	nextAttempt time.Time
}

type reconcileBackoff struct {
	mu      sync.Mutex
	entries map[string]backoffEntry
	jitter  func() float64
}

func newReconcileBackoff() *reconcileBackoff {
	return &reconcileBackoff{entries: make(map[string]backoffEntry), jitter: rand.Float64}
}

func (b *reconcileBackoff) due(symbol string, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	entry, ok := b.entries[symbol]
	return !ok || !now.Before(entry.nextAttempt)
}

func (b *reconcileBackoff) recordFailure(symbol string, now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	entry := b.entries[symbol]
	entry.failures++
	entry.nextAttempt = now.Add(b.delay(entry.failures))
	b.entries[symbol] = entry
}

func (b *reconcileBackoff) recordSuccess(symbol string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.entries, symbol)
}

func (b *reconcileBackoff) delay(failures int) time.Duration {
	wait := reconcileBackoffMax
	if failures <= reconcileBackoffCapAt {
		wait = min(reconcileBackoffBase<<(failures-1), reconcileBackoffMax)
	}
	factor := 1 + (b.jitter()*2-1)*reconcileBackoffJitter
	return time.Duration(float64(wait) * factor)
}
