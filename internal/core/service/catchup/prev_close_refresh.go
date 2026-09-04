package catchup

import (
	"context"
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

// RefreshPrevClose calcula el prevClose (cierre de la subasta de la sesion
// anterior) SOLO para los simbolos cuyo prev_close_updated_at quedo fuera
// de la ventana de mantenimiento actual -- el guard por-simbolo con fecha
// evita recalcular los 13k en cada reinicio (corre en el backfill, despues
// del barrido D1 y antes de H1, ver universe_cycle.go). La lectura
// (GetPreviousSessionCloseBatch) y la escritura (UpsertPrevCloseBatch) son
// cada una UN SOLO round trip para todo el lote stale -- nunca un query o
// upsert por simbolo.
func RefreshPrevClose(ctx context.Context, candles out.CandleRepository, fundamentals out.FundamentalsRepository, windowStart time.Time) error {
	stale, err := fundamentals.GetSymbolsWithStalePrevClose(ctx, windowStart)
	if err != nil {
		return fmt.Errorf("listing symbols with stale prev close: %w", err)
	}
	if len(stale) == 0 {
		log.Info().Msg("prev close refresh: all symbols already done for this maintenance window, skipping")
		return nil
	}

	start := time.Now()
	closes, err := candles.GetPreviousSessionCloseBatch(ctx, stale, time.Now())
	if err != nil {
		return fmt.Errorf("loading previous session close batch: %w", err)
	}

	attemptedOnly := make([]string, 0, len(stale))
	for _, symbol := range stale {
		if _, ok := closes[symbol]; !ok {
			attemptedOnly = append(attemptedOnly, symbol)
		}
	}

	if err := fundamentals.UpsertPrevCloseBatch(ctx, closes, attemptedOnly); err != nil {
		return fmt.Errorf("upserting prev close batch: %w", err)
	}

	log.Info().Int("found", len(closes)).Int("attempted_only", len(attemptedOnly)).Dur("elapsed", time.Since(start)).Msg("prev close refresh finished")
	return nil
}
