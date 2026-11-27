package tastytrade

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/rs/zerolog/log"
)

func (p *CandlePool) swapLiveSymbolChannel(ctx context.Context, symbol string, oldCh *pooledChannel, from time.Time, epoch uint64, now time.Time) (*pooledChannel, error) {
	from = p.resolveResume(symbol, from, now)
	newCh, err := p.claimLiveChannelExcluding(ctx, symbol, oldCh)
	if err != nil {
		return nil, err
	}
	key := candleKey(symbol, domain.M1)
	if !p.isLiveEpochActive(epoch) {
		newCh.release(key)
		return nil, context.Canceled
	}
	p.unsubscribeOtherLiveChannels(symbol, key, newCh)
	dispatchID := p.registerDispatch(symbol, domain.M1, "live", func(ev rawCandleEvent) { p.handleLiveEvent(symbol, ev) })
	if err := newCh.channel.subscribeLive(symbol, domain.M1, from); err != nil {
		p.unregisterDispatchIfCurrent(symbol, domain.M1, dispatchID, "live")
		newCh.release(key)
		return nil, err
	}
	if err := newCh.channel.subscribeLiveDayVolume(symbol); err != nil {
		log.Warn().Err(err).Str("symbol", symbol).Msg("failed to subscribe live day volume (Trade/TradeETH), falling back to batch fetch")
	}
	p.noteSubscribed(symbol, now)
	return newCh, nil
}

func (p *CandlePool) isLiveEpochActive(epoch uint64) bool {
	p.liveMu.Lock()
	defer p.liveMu.Unlock()
	return p.liveStopping == 0 && p.liveEpoch == epoch
}

func (p *CandlePool) unsubscribeOtherLiveChannels(symbol, key string, keep *pooledChannel) {
	for _, ch := range p.allocator.allChannels() {
		if ch != keep && channelHasKey(ch, key) {
			_ = ch.channel.unsubscribe(symbol, domain.M1)
			_ = ch.channel.unsubscribeLiveDayVolume(symbol)
			ch.release(key)
		}
	}
}

func channelHasKey(ch *pooledChannel, key string) bool {
	ch.mu.Lock()
	_, ok := ch.symbols[key]
	ch.mu.Unlock()
	return ok
}
