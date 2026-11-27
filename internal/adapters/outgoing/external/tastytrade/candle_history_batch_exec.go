package tastytrade

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/rs/zerolog/log"
)

func channelLogIDs(ch *pooledChannel) (int64, int) {
	if ch == nil || ch.channel == nil {
		return 0, 0
	}
	var connID int64
	if ch.channel.client != nil {
		connID = ch.channel.client.id
	}
	return connID, ch.channel.id
}

func logHistoryBatchFinished(ch *pooledChannel, tf domain.Timeframe, stats historyBatchStats, cut bool) {
	connID, channelID := channelLogIDs(ch)
	log.Info().
		Str("timeframe", string(tf)).
		Int64("connID", connID).
		Int("channel", channelID).
		Int("requested", stats.requested).
		Int("withData", stats.withData).
		Int("signaled", stats.signaled).
		Int("signaledWithData", stats.signaledWithData).
		Int("snipped", stats.snipped).
		Int("withoutData", stats.withoutData).
		Bool("cut", cut).
		Msg("history batch finished")
}

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
	err := waitForData(ctx, collector.settled, batchHistoryWait(froms))
	if err != nil && !errors.Is(err, domain.ErrPartialHistory) {
		return nil, err
	}
	result := collector.complete()
	cut := errors.Is(err, domain.ErrPartialHistory)
	logHistoryBatchFinished(ch, tf, collector.stats(active, result), cut)
	return result, err
}
