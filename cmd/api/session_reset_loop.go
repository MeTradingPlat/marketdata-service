package main

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs"
	"github.com/rs/zerolog/log"
)

// StartSessionResetLoop rota el access token una vez al dia a la hora
// configurada (default 00:05 UTC, que en horario de invierno cae a las 19:05
// ET, todavia dentro del post-market). No toca las conexiones DxLink:
// ResetSessions ya no hace DELETE /sessions, asi que cerrarlas solo dejaba el
// live apagado hasta el rollout nocturno (confirmado en vivo el 2026-11-25).
// El pool arranca desde cero en el ciclo nocturno (ver universe_cycle.go).
func StartSessionResetLoop(ctx context.Context, cfg *configs.Config, resetter out.SessionResetter) {
	go func() {
		for {
			next := nextResetTime(time.Now(), cfg.SessionResetHour, cfg.SessionResetMinute)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Until(next)):
			}
			runScheduledReset(ctx, resetter)
		}
	}()
}

func runScheduledReset(ctx context.Context, resetter out.SessionResetter) {
	if err := resetter.ResetSessions(ctx); err != nil {
		log.Error().Err(err).Msg("scheduled session reset failed")
		return
	}
	log.Info().Msg("scheduled session reset finished")
}

// nextResetTime devuelve la proxima ocurrencia de la hora configurada en
// UTC -- si ya paso hoy, la de manana.
func nextResetTime(now time.Time, hour, minute int) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.UTC)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}
