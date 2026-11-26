package tastytrade

import (
	"sort"
	"time"

	"github.com/rs/zerolog/log"
)

func (p *CandlePool) LogLiveHealth(now time.Time) {
	p.liveMu.Lock()
	subs := make([]string, 0, len(p.liveSubs))
	for sym, cb := range p.liveSubs {
		if cb != nil {
			subs = append(subs, sym)
		}
	}
	p.liveMu.Unlock()
	sort.Strings(subs)

	pooled := p.pooledLiveSymbols()
	var noDispatch, noChannel, staleActive []string
	regular := isRegularSessionET(now)
	for _, sym := range subs {
		if !p.hasLiveDispatch(sym) {
			noDispatch = append(noDispatch, sym)
		}
		if _, ok := pooled[sym]; !ok {
			noChannel = append(noChannel, sym)
		}
		if regular && p.isStaleWithActivity(sym, now) {
			staleActive = append(staleActive, sym)
		}
	}
	log.Info().
		Int("live_subs", len(subs)).
		Int("live_without_dispatch", len(noDispatch)).
		Strs("live_without_dispatch_sample", sampleUpTo10(noDispatch)).
		Int("live_without_channel", len(noChannel)).
		Strs("live_without_channel_sample", sampleUpTo10(noChannel)).
		Int("live_stale_with_activity", len(staleActive)).
		Strs("live_stale_with_activity_sample", sampleUpTo10(staleActive)).
		Msg("live health")
}

func sampleUpTo10(items []string) []string {
	if len(items) > 10 {
		return items[:10]
	}
	return items
}
