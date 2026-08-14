package main

import (
	"context"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/rs/zerolog/log"
)

// backfillPhase corre un timeframe para todos los simbolos antes de pasar
// al siguiente, en vez de encadenar D1+H1+M1 por simbolo -- deja libre el
// canal/conexion de cada fetch puntual antes de arrancar el siguiente lote,
// el mismo patron que va a necesitar el universo completo de simbolos.
func backfillPhase(ctx context.Context, ingest in.IngestCandlesService, symbolList []string, tf domain.Timeframe) {
	for _, symbol := range symbolList {
		if err := ingest.Backfill(ctx, symbol, tf); err != nil {
			log.Error().Err(err).Str("symbol", symbol).Str("timeframe", string(tf)).Msg("failed to backfill history")
			continue
		}
		log.Info().Str("symbol", symbol).Str("timeframe", string(tf)).Msg("backfill finished")
	}
}
