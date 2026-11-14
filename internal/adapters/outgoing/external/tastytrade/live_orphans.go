package tastytrade

import (
	"sort"
)

func (p *CandlePool) orphanedLiveSymbols() []string {
	p.liveMu.Lock()
	live := make([]string, 0, len(p.liveSubs))
	for sym := range p.liveSubs {
		live = append(live, sym)
	}
	p.liveMu.Unlock()

	pooled := make(map[string]struct{})
	for _, ch := range p.allocator.allChannels() {
		for _, sym := range ch.liveSymbols() {
			pooled[sym] = struct{}{}
		}
	}

	var orphans []string
	for _, sym := range live {
		if _, ok := pooled[sym]; !ok {
			orphans = append(orphans, sym)
		}
	}
	sort.Strings(orphans)
	return orphans
}
