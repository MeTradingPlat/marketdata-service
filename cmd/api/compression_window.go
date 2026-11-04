package main

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

func pauseCompression(ctx context.Context, candles out.CandleRepository) {
	cs, ok := candles.(out.CompressionScheduler)
	if !ok {
		return
	}
	if err := cs.SetCompressionScheduled(ctx, false); err != nil {
		log.Warn().Err(err).Msg("failed to pause compression for sweep")
		return
	}
	log.Info().Msg("compression paused for sweep")
}

func resumeCompression(candles out.CandleRepository) {
	cs, ok := candles.(out.CompressionScheduler)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := cs.SetCompressionScheduled(ctx, true); err != nil {
		log.Warn().Err(err).Msg("failed to resume compression after sweep")
		return
	}
	log.Info().Msg("compression resumed")
}
