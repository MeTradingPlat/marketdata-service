package aggregation

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

const postMidnightDelay = 5 * time.Minute

// StartForwardAggregation deriva H1/D1 desde M1 una vez al dia, poco despues
// de que cierra la barra D1 (medianoche UTC) -- no falta hacerlo mas seguido
// porque quien necesita datos en vivo lee M1 directo; H1/D1 derivadas son
// para historico/backtest, no para el path en tiempo real.
func StartForwardAggregation(ctx context.Context, repo out.CandleRepository) {
	go func() {
		runOnce(ctx, repo)
		for {
			wait := time.Until(nextRunAt(time.Now().UTC()))
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
				runOnce(ctx, repo)
			}
		}
	}()
}

func nextRunAt(now time.Time) time.Time {
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	next := midnight.Add(postMidnightDelay)
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

func runOnce(ctx context.Context, repo out.CandleRepository) {
	if err := repo.AggregateH1(ctx); err != nil {
		log.Error().Err(err).Msg("failed to aggregate H1 from M1")
	}
	if err := repo.AggregateD1(ctx); err != nil {
		log.Error().Err(err).Msg("failed to aggregate D1 from M1")
	}
}
