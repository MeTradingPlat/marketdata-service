package tastytrade

import (
	"sort"
	"time"
)

func (p *CandlePool) canRepairSilentSymbol(symbol string, now time.Time) bool {
	p.activityMu.Lock()
	defer p.activityMu.Unlock()
	p.ensureActivityMaps()
	history := p.silentHealHistory[symbol]
	cutoff := now.Add(-silentHealRateWindow)
	kept := history[:0]
	for _, ts := range history {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	p.silentHealHistory[symbol] = kept
	return len(kept) < silentHealMaxPerHour
}

func (p *CandlePool) recordSilentRepair(symbol string, now time.Time) {
	p.activityMu.Lock()
	p.ensureActivityMaps()
	p.silentHealHistory[symbol] = append(p.silentHealHistory[symbol], now)
	p.activityMu.Unlock()
}

func (p *CandlePool) dayVolumeForRank(symbol string) float64 {
	entry, ok := p.LiveDayVolume(symbol)
	if !ok || entry.Regular == nil {
		return 0
	}
	return *entry.Regular
}

func (p *CandlePool) silentHealCandidates(now time.Time, limit int) []string {
	p.liveMu.Lock()
	subs := make([]string, 0, len(p.liveSubs))
	for sym, cb := range p.liveSubs {
		if cb != nil {
			subs = append(subs, sym)
		}
	}
	p.liveMu.Unlock()

	candidates := make([]string, 0, len(subs))
	for _, sym := range subs {
		if p.isStaleWithActivity(sym, now) && p.canRepairSilentSymbol(sym, now) {
			candidates = append(candidates, sym)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		vi, vj := p.dayVolumeForRank(candidates[i]), p.dayVolumeForRank(candidates[j])
		if vi != vj {
			return vi > vj
		}
		return candidates[i] < candidates[j]
	})
	if limit >= 0 && len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return candidates
}
