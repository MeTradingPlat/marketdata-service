package tastytrade

import (
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
