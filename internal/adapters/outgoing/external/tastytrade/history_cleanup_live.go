package tastytrade

import "github.com/MeTradingPlat/marketdata-service/internal/core/domain"

func (p *CandlePool) hostsLiveSubscription(ch *pooledChannel, symbol string, tf domain.Timeframe) bool {
	if tf != domain.M1 || !p.LiveSubscribed(symbol) {
		return false
	}
	key := candleKey(symbol, tf)
	for _, other := range p.allocator.allChannels() {
		if other != ch && channelHasKey(other, key) {
			return false
		}
	}
	return true
}

func (p *CandlePool) filterCleanupSymbols(ch *pooledChannel, active []string, tf domain.Timeframe) []string {
	var toRemove []string
	for _, symbol := range active {
		if !p.hostsLiveSubscription(ch, symbol, tf) {
			toRemove = append(toRemove, symbol)
		}
	}
	return toRemove
}
