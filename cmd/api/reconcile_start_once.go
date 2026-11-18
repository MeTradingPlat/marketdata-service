package main

import (
	"context"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/rs/zerolog/log"
)

func startLiveOnce(ingest in.IngestCandlesService) startAttempt {
	return func(ctx context.Context, symbol string) error {
		err := ingest.StreamLive(ctx, symbol)
		if err != nil && ctx.Err() == nil {
			log.Error().Err(err).Str("symbol", symbol).Msg("live reconcile: failed to start live candle stream")
			recordGiveUp(symbol)
		}
		return err
	}
}
