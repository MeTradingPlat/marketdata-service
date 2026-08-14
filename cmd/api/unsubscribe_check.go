package main

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/rs/zerolog/log"
)

// runUnsubscribeCheck repite backfill+unsubscribe muchas veces para el
// MISMO simbolo -- si la baja de suscripcion no funciona de verdad, las
// suscripciones fantasma se acumulan en el mismo canal y las ultimas
// vueltas se vuelven mas lentas que las primeras (el mismo sintoma que
// revelo el bug original). Diagnostico, no flujo de produccion.
func runUnsubscribeCheck(ctx context.Context, ingest in.IngestCandlesService, symbol string, rounds int) {
	log.Info().Str("symbol", symbol).Int("rounds", rounds).Msg("starting unsubscribe leak check")
	for i := 1; i <= rounds; i++ {
		start := time.Now()
		if err := ingest.Backfill(ctx, symbol, domain.D1); err != nil {
			log.Error().Err(err).Int("round", i).Msg("unsubscribe check round failed")
			continue
		}
		log.Info().Int("round", i).Dur("elapsed", time.Since(start)).Msg("unsubscribe check round")
	}
	log.Info().Str("symbol", symbol).Msg("unsubscribe leak check finished")
}
