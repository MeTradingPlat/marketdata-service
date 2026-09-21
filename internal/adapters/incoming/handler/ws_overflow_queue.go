package handler

import (
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	maxOverflowMessages = 200_000
	overflowLogInterval = 10 * time.Second
)

type overflowQueue struct {
	mu      sync.Mutex
	items   []any
	max     int
	queued  int
	dropped int
	lastLog time.Time
}

func newOverflowQueue(max int) *overflowQueue {
	return &overflowQueue{max: max}
}

func (q *overflowQueue) push(v any) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	defer q.logLocked()
	if len(q.items) >= q.max {
		q.dropped++
		return false
	}
	q.items = append(q.items, v)
	q.queued++
	return true
}

func (q *overflowQueue) take() []any {
	q.mu.Lock()
	defer q.mu.Unlock()
	items := q.items
	q.items = nil
	return items
}

func (q *overflowQueue) logLocked() {
	if time.Since(q.lastLog) < overflowLogInterval {
		return
	}
	q.lastLog = time.Now()
	if q.dropped > 0 {
		log.Error().Int("dropped", q.dropped).Msg("ws session overflow queue full, closed candles dropped")
	}
	if q.queued > 0 {
		log.Warn().Int("queued", q.queued).Msg("ws session outbound queue overflowed, closed candles queued behind the writer")
	}
	q.queued, q.dropped = 0, 0
}
