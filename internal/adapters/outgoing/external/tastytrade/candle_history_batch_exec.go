package tastytrade

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func (p *CandlePool) batchCleanup(ch *pooledChannel, active []string, tf domain.Timeframe, dispatchIDs map[string]uint64) func() {
	return func() {
		toRemove := p.filterCleanupSymbols(ch, active, tf)
		if len(toRemove) > 0 {
			_ = ch.channel.unsubscribeHistoryBatch(toRemove, tf)
			for _, symbol := range toRemove {
				ch.release(candleKey(symbol, tf))
			}
		}
		time.AfterFunc(unsubscribeDrainPeriod, func() {
			p.unregisterBatchDispatch(active, tf, dispatchIDs)
		})
	}
}

func (p *CandlePool) fetchBatchOnChannel(ctx context.Context, ch *pooledChannel, active []string, tf domain.Timeframe, froms map[string]time.Time, collector *batchHistoryCollector, dispatchIDs map[string]uint64) (map[string][]domain.Candle, error) {
	cleanup := p.batchCleanup(ch, active, tf, dispatchIDs)
	defer cleanup()
	if err := ch.channel.subscribeHistoryBatch(active, tf, froms); err != nil {
		return nil, fmt.Errorf("subscribing history batch: %w", err)
	}
	if err := waitForData(ctx, collector.settled, batchHistoryWait(froms)); err != nil {
		if errors.Is(err, domain.ErrPartialHistory) {
			return collector.complete(), err
		}
		return nil, err
	}
	return collector.complete(), nil
}
