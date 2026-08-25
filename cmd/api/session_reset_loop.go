package main

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/external/tastytrade"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs"
	"github.com/rs/zerolog/log"
)

// StartSessionResetLoop cierra proactivamente todas las sesiones de
// TastyTrade una vez al dia a la hora configurada (default 22:15 UTC, con el
// mercado US ya cerrado) -- las sesiones huerfanas de swaps/kills se acumulan
// server-side y saturan el limite del usuario ("number of user sessions has
// exceeded the configured limit"), asi que el reset diario garantiza una mesa
// limpia cada manana aunque la limpieza reactiva del reconnect hubiera
// fallado el dia anterior. Se ejecuta con el mercado cerrado para no cortar
// streams en vivo en horario de trading.
func StartSessionResetLoop(ctx context.Context, cfg *configs.Config, oauth *tastytrade.OAuth, gateway out.MarketDataGateway) {
	go func() {
		for {
			next := nextResetTime(time.Now(), cfg.SessionResetHour, cfg.SessionResetMinute)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Until(next)):
			}
			if err := oauth.ResetSessions(ctx); err != nil {
				log.Error().Err(err).Msg("scheduled session reset failed")
				continue
			}
			// El DELETE /sessions mato tambien las conexiones vivas de este
			// proceso: el pool reabre al proximo uso con el token fresco.
			gateway.ResetLiveConnections()
			log.Info().Msg("scheduled session reset finished")
		}
	}()
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
