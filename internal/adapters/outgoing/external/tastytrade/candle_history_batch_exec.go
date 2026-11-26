package tastytrade

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func (p *CandlePool) occupyBatchChannel(ch *pooledChannel, active []string, tf domain.Timeframe, dispatchIDs map[string]uint64) func() {
	for _, symbol := range active {
		ch.occupy(candleKey(symbol, tf))
	}
	return func() {
		for _, symbol := range active {
			ch.release(candleKey(symbol, tf))
		}
		_ = ch.channel.unsubscribeHistoryBatch(active, tf)
		time.AfterFunc(unsubscribeDrainPeriod, func() {
			for _, symbol := range active {
				p.unregisterDispatchIfCurrent(symbol, tf, dispatchIDs[symbol], "history-batch")
			}
		})
	}
}

func (p *CandlePool) fetchBatchOnChannel(ctx context.Context, ch *pooledChannel, active []string, tf domain.Timeframe, froms map[string]time.Time, collector *batchHistoryCollector, dispatchIDs map[string]uint64) (map[string][]domain.Candle, error) {
	cleanup := p.occupyBatchChannel(ch, active, tf, dispatchIDs)
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
