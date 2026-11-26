package tastytrade

import (
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

const discardLogInterval = time.Minute

type sampledDiscardCounter struct {
	count     atomic.Int64
	lastLogNs atomic.Int64
}

func (c *sampledDiscardCounter) record(symbol string, now time.Time, msg string) int64 {
	total := c.count.Add(1)
	nowNs := now.UnixNano()
	lastNs := c.lastLogNs.Load()
	if nowNs-lastNs >= discardLogInterval.Nanoseconds() && c.lastLogNs.CompareAndSwap(lastNs, nowNs) {
		log.Warn().Int64("total", total).Str("sample_symbol", symbol).Msg(msg)
	}
	return total
}

func (c *sampledDiscardCounter) total() int64 {
	return c.count.Load()
}
