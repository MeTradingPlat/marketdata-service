package tastytrade

import (
	"context"
	"errors"
	"time"

	"github.com/rs/zerolog/log"
)

// orphanReattachGrace evita pisar una reconexion asincrona en curso antes de
// reenganchar un simbolo huerfano.
const orphanReattachGrace = 2 * time.Minute

const orphanReattachMaxConsecutiveFailures = 3

func (p *CandlePool) ReattachOrphans(ctx context.Context, now time.Time, limit int) int {
	if limit <= 0 || !p.CanSubscribeLive() {
		return 0
	}
	orphans := p.orphanedLiveSymbols()
	candidates := p.selectOrphanCandidates(orphans, now, limit)
	reattached := p.reattachCandidates(ctx, candidates, now)
	if reattached > 0 {
		log.Info().Int("reattached", reattached).Int("orphans", len(orphans)).Msg("orphan live symbols reattached")
	}
	return reattached
}

func (p *CandlePool) selectOrphanCandidates(orphans []string, now time.Time, limit int) []string {
	var candidates []string
	p.activityMu.Lock()
	p.ensureActivityMaps()
	p.pruneStaleOrphansLocked(orphans)
	for _, sym := range orphans {
		since, ok := p.orphanSince[sym]
		if !ok {
			p.orphanSince[sym] = now
			continue
		}
		if now.Sub(since) >= orphanReattachGrace && len(candidates) < limit {
			candidates = append(candidates, sym)
		}
	}
	p.activityMu.Unlock()
	return candidates
}

func (p *CandlePool) pruneStaleOrphansLocked(orphans []string) {
	orphanSet := make(map[string]struct{}, len(orphans))
	for _, sym := range orphans {
		orphanSet[sym] = struct{}{}
	}
	for sym := range p.orphanSince {
		if _, ok := orphanSet[sym]; !ok {
			delete(p.orphanSince, sym)
		}
	}
}

func (p *CandlePool) reattachCandidates(ctx context.Context, candidates []string, now time.Time) int {
	reattached := 0
	consecutiveFailures := 0
	for _, sym := range candidates {
		if ctx.Err() != nil || consecutiveFailures >= orphanReattachMaxConsecutiveFailures {
			break
		}
		ok, failed := p.reattachOrphanSymbol(ctx, sym, now)
		if failed {
			consecutiveFailures++
			continue
		}
		if ok {
			reattached++
			consecutiveFailures = 0
		}
	}
	return reattached
}

func (p *CandlePool) reattachOrphanSymbol(ctx context.Context, sym string, now time.Time) (bool, bool) {
	cb, tick, epoch, stopping := p.readLiveSubForHeal(sym)
	if stopping {
		return false, false
	}
	if cb == nil {
		p.forgetOrphan(sym)
		return false, false
	}
	if err := p.subscribeLiveAtEpoch(ctx, sym, p.lastClosedTimestamp(sym), cb, tick, epoch); err != nil {
		if errors.Is(err, context.Canceled) || ctx.Err() != nil {
			return false, false
		}
		p.activityMu.Lock()
		p.ensureActivityMaps()
		p.orphanSince[sym] = now
		p.activityMu.Unlock()
		log.Warn().Err(err).Str("symbol", sym).Msg("failed to reattach orphan live symbol")
		return false, true
	}
	p.forgetOrphan(sym)
	return true, false
}

func (p *CandlePool) lastClosedTimestamp(sym string) time.Time {
	var resumeFrom time.Time
	p.currentMu.Lock()
	if last, ok := p.lastClosed[sym]; ok {
		resumeFrom = last.Timestamp
	}
	p.currentMu.Unlock()
	return resumeFrom
}

func (p *CandlePool) forgetOrphan(symbol string) {
	p.activityMu.Lock()
	delete(p.orphanSince, symbol)
	delete(p.pendingRevive, symbol)
	p.activityMu.Unlock()
}
