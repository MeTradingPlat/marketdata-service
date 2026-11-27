package tastytrade

import (
	"context"
	"time"
)

var reviveTimeout = 3 * time.Minute

func (p *CandlePool) StartReviveSilentLive(ctx context.Context, now time.Time) bool {
	if !p.reviveRunning.CompareAndSwap(false, true) {
		return false
	}
	go func() {
		defer p.reviveRunning.Store(false)
		runCtx, cancel := context.WithTimeout(ctx, reviveTimeout)
		defer cancel()
		p.ReviveSilentLive(runCtx, now)
	}()
	return true
}
