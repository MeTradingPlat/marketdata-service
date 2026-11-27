package tastytrade

import "github.com/MeTradingPlat/marketdata-service/internal/core/domain"

func (p *CandlePool) shouldSkipLiveM1History(symbol string, tf domain.Timeframe) bool {
	return tf == domain.M1 && p.LiveSubscribed(symbol)
}

func (p *CandlePool) registerHistoryDispatch(ch *pooledChannel, symbol string, tf domain.Timeframe, key string, handler func(rawCandleEvent)) uint64 {
	dispatchID := p.registerDispatch(symbol, tf, "history", handler)
	if dispatchID == 0 && !p.hostsLiveSubscription(ch, symbol, tf) {
		ch.release(key)
	}
	return dispatchID
}
