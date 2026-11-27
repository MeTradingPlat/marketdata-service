package tastytrade

import (
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

type subChangeCounter struct {
	mu          sync.Mutex
	interval    time.Duration
	now         func() time.Time
	emit        func(changes, adds, removes, windowSeconds int)
	windowStart time.Time
	adds        int
	removes     int
}

var dxLinkSubChanges = newSubChangeCounter(time.Minute, time.Now, emitSubChangesLog)

func newSubChangeCounter(interval time.Duration, now func() time.Time, emit func(changes, adds, removes, windowSeconds int)) *subChangeCounter {
	return &subChangeCounter{
		interval: interval,
		now:      now,
		emit:     emit,
	}
}

func (c *subChangeCounter) record(adds, removes int) {
	if adds == 0 && removes == 0 {
		return
	}
	now := c.now()
	c.mu.Lock()
	if c.windowStart.IsZero() {
		c.windowStart = now
	}
	c.adds += adds
	c.removes += removes
	elapsed := now.Sub(c.windowStart)
	if elapsed < c.interval {
		c.mu.Unlock()
		return
	}
	totalAdds, totalRemoves := c.adds, c.removes
	c.adds, c.removes = 0, 0
	c.windowStart = now
	c.mu.Unlock()
	c.emit(totalAdds+totalRemoves, totalAdds, totalRemoves, int(elapsed.Seconds()))
}

func emitSubChangesLog(changes, adds, removes, windowSeconds int) {
	log.Info().
		Int("changes", changes).
		Int("adds", adds).
		Int("removes", removes).
		Int("windowSeconds", windowSeconds).
		Msg("dxlink subscription changes")
}
