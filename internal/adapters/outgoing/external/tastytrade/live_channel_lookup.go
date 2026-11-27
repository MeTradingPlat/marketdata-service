package tastytrade

import (
	"context"
	"fmt"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func (p *CandlePool) claimLiveChannelExcluding(ctx context.Context, symbol string, exclude *pooledChannel) (*pooledChannel, error) {
	key := candleKey(symbol, domain.M1)
	for attempt := 0; attempt < 3; attempt++ {
		ch, err := p.allocator.allocateExcluding(ctx, exclude, key)
		if err != nil {
			return nil, err
		}
		if p.allocator.owns(ch) {
			return ch, nil
		}
		ch.release(key)
	}
	return nil, fmt.Errorf("failed to claim distinct live channel for %s after 3 attempts", symbol)
}

func (p *CandlePool) liveChannelFor(symbol string) *pooledChannel {
	key := candleKey(symbol, domain.M1)
	for _, ch := range p.allocator.allChannels() {
		if channelHasKey(ch, key) {
			return ch
		}
	}
	return nil
}
