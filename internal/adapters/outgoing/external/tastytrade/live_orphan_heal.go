package tastytrade

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

// orphanReattachGrace evita pisar una reconexion asincrona en curso antes de
// reenganchar un simbolo huerfano.
const orphanReattachGrace = 2 * time.Minute

const orphanReattachMaxConsecutiveFailures = 3

func (p *CandlePool) ReattachOrphans(ctx context.Context, now time.Time, limit int) int {
	if limit <= 0 || !p.Connected() {
		return 0
	}
	orphans := p.orphanedLiveSymbols()
	orphanSet := make(map[string]struct{}, len(orphans))
	for _, sym := range orphans {
		orphanSet[sym] = struct{}{}
	}
	var candidates []string
	p.activityMu.Lock()
	p.ensureActivityMaps()
	for sym := range p.orphanSince {
		if _, ok := orphanSet[sym]; !ok {
			delete(p.orphanSince, sym)
		}
	}
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
	reattached := 0
	consecutiveFailures := 0
	for _, sym := range candidates {
		if consecutiveFailures >= orphanReattachMaxConsecutiveFailures {
			break
		}
		p.liveMu.Lock()
		cb, tick := p.liveSubs[sym], p.liveTicks[sym]
		p.liveMu.Unlock()
		if cb == nil {
			p.forgetOrphan(sym)
			continue
		}
		var resumeFrom time.Time
		p.currentMu.Lock()
		if last, ok := p.lastClosed[sym]; ok {
			resumeFrom = last.Timestamp
		}
		p.currentMu.Unlock()
		if err := p.SubscribeLive(ctx, sym, resumeFrom, cb, tick); err != nil {
			p.activityMu.Lock()
			p.ensureActivityMaps()
			p.orphanSince[sym] = now
			p.activityMu.Unlock()
			log.Warn().Err(err).Str("symbol", sym).Msg("failed to reattach orphan live symbol")
			consecutiveFailures++
			continue
		}
		p.forgetOrphan(sym)
		reattached++
		consecutiveFailures = 0
	}
	if reattached > 0 {
		log.Info().Int("reattached", reattached).Int("orphans", len(orphans)).Msg("orphan live symbols reattached")
	}
	return reattached
}

func (p *CandlePool) forgetOrphan(symbol string) {
	p.activityMu.Lock()
	delete(p.orphanSince, symbol)
	delete(p.pendingRevive, symbol)
	p.activityMu.Unlock()
}
