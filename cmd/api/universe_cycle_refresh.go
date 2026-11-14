package main

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

const (
	refreshMaxAttempts      = 3
	refreshRetryDelay       = 3 * time.Minute
	liveRolloutSessionGrace = 60 * time.Second
)

// refreshWithRetry reintenta un refresh idempotente del barrido nocturno.
func refreshWithRetry(name string, fn func() error) {
	for attempt := 1; attempt <= refreshMaxAttempts; attempt++ {
		if err := fn(); err != nil {
			log.Error().Err(err).Int("attempt", attempt).Str("refresh", name).Msg("nightly refresh failed")
			if attempt < refreshMaxAttempts {
				time.Sleep(refreshRetryDelay)
			}
			continue
		}
		return
	}
}

// refreshFundamentalsOnce corre el refresh solo si no se completo ya en la
// ventana de mantenimiento actual -- la marca fundamental_refresh_log
// sobrevive reinicios, asi un redeploy a mitad de dia no recalcula los
// datos diarios (beta, earnings, externos) que ya se calcularon tras el
// cierre. La ventana nocturna siempre los recalcula: su done_at quedo en
// la ventana ANTERIOR (el arranque de ventana se avanza cada cierre), asi
// que la comparacion no confunde "lo de ayer" con "lo de hoy" -- un done_at
// viejo es anterior a la ventana actual y dispara el recalculo.
func refreshFundamentalsOnce(ctx context.Context, fundamentals out.FundamentalsRepository, step string, windowStart time.Time, fn func() error) {
	doneAt, done, err := fundamentals.StepDoneAt(ctx, step)
	if err != nil {
		log.Error().Err(err).Str("step", step).Msg("fundamental refresh log check failed, running anyway")
	} else if done && !doneAt.Before(windowStart) {
		log.Info().Str("step", step).Time("done_at", doneAt).Msg("fundamental refresh already done for this maintenance window, skipping")
		return
	}
	refreshWithRetry(step, func() error {
		if err := fn(); err != nil {
			return err
		}
		return fundamentals.RecordStepDone(ctx, step, time.Now())
	})
}
