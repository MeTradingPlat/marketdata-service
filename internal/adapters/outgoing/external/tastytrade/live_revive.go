package tastytrade

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/rs/zerolog/log"
)

func (p *CandlePool) ReviveSilentLive(ctx context.Context, now time.Time) {
	var targets []string
	p.activityMu.Lock()
	for sym, st := range p.pendingRevive {
		if last := p.lastEventAt[sym]; shouldDropRevive(st, last, now) {
			delete(p.pendingRevive, sym)
		} else if shouldRevive(st, last, now) && len(targets) < reviveMaxPerTick {
			targets = append(targets, sym)
		}
	}
	p.activityMu.Unlock()
	revived := 0
	for _, sym := range targets {
		p.liveMu.Lock()
		cb, tick := p.liveSubs[sym], p.liveTicks[sym]
		p.liveMu.Unlock()
		if cb == nil {
			p.activityMu.Lock()
			delete(p.pendingRevive, sym)
			p.activityMu.Unlock()
			continue
		}
		p.allocator.releaseKey(candleKey(sym, domain.M1))
		var resumeFrom time.Time
		p.currentMu.Lock()
		if last, ok := p.lastClosed[sym]; ok {
			resumeFrom = last.Timestamp
		}
		p.currentMu.Unlock()
		err := p.SubscribeLive(ctx, sym, resumeFrom, cb, tick)
		p.activityMu.Lock()
		st := p.pendingRevive[sym]
		st.attempts++
		st.lastAttemptAt = now
		p.pendingRevive[sym] = st
		p.activityMu.Unlock()
		if err != nil {
			log.Warn().Err(err).Str("symbol", sym).Msg("failed to revive silent live symbol")
			continue
		}
		revived++
		log.Info().Str("symbol", sym).Msg("revived silent live symbol")
	}
	if revived > 0 {
		log.Info().Int("count", revived).Msg("revived silent live symbols")
	}
}
