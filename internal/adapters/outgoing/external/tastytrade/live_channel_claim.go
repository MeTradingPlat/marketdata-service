package tastytrade

import (
	"context"
	"fmt"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func (p *CandlePool) claimLiveChannel(ctx context.Context, symbol string) (*pooledChannel, error) {
	key := candleKey(symbol, domain.M1)
	for attempt := 0; attempt < 3; attempt++ {
		ch, err := p.allocator.allocate(ctx)
		if err != nil {
			return nil, err
		}
		ch.occupy(key)
		if p.allocator.owns(ch) {
			return ch, nil
		}
		ch.release(key)
	}
	return nil, fmt.Errorf("failed to claim owned live channel for %s after 3 attempts", symbol)
}
