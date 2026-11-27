package tastytrade

import (
	"context"
	"errors"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/rs/zerolog/log"
)

func (p *CandlePool) DropExpiredRevive(now time.Time) {
	p.activityMu.Lock()
	for sym, st := range p.pendingRevive {
		if shouldDropRevive(st, p.lastEventAt[sym], now) {
			delete(p.pendingRevive, sym)
		}
	}
	p.activityMu.Unlock()
}

const reviveMaxConsecutiveFailures = 3

func (p *CandlePool) ReviveSilentLive(ctx context.Context, now time.Time) {
	targets := p.selectReviveTargets(now)
	revived := 0
	consecutiveFailures := 0
	for _, sym := range targets {
		if ctx.Err() != nil || consecutiveFailures >= reviveMaxConsecutiveFailures {
			break
		}
		ok, failed := p.reviveSilentSymbol(ctx, sym, now)
		if failed {
			consecutiveFailures++
			continue
		}
		if ok {
			consecutiveFailures = 0
			revived++
		}
	}
	if revived > 0 {
		log.Info().Int("count", revived).Msg("revived silent live symbols")
	}
}

func (p *CandlePool) selectReviveTargets(now time.Time) []string {
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
	return targets
}

func (p *CandlePool) reviveSilentSymbol(ctx context.Context, sym string, now time.Time) (bool, bool) {
	cb, tick, epoch, stopping := p.readLiveSubForHeal(sym)
	if stopping {
		return false, false
	}
	if cb == nil {
		p.activityMu.Lock()
		delete(p.pendingRevive, sym)
		p.activityMu.Unlock()
		return false, false
	}
	p.allocator.releaseKey(candleKey(sym, domain.M1))
	err := p.subscribeLiveAtEpoch(ctx, sym, p.lastClosedTimestamp(sym), cb, tick, epoch)
	if errors.Is(err, context.Canceled) || (err != nil && ctx.Err() != nil) {
		return false, false
	}
	p.recordReviveAttempt(sym, now)
	if err != nil {
		log.Warn().Err(err).Str("symbol", sym).Msg("failed to revive silent live symbol")
		return false, true
	}
	log.Info().Str("symbol", sym).Msg("revived silent live symbol")
	return true, false
}

func (p *CandlePool) recordReviveAttempt(sym string, now time.Time) {
	p.activityMu.Lock()
	st := p.pendingRevive[sym]
	st.attempts++
	st.lastAttemptAt = now
	p.pendingRevive[sym] = st
	p.activityMu.Unlock()
}
