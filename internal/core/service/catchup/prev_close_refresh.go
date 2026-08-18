package catchup

import (
	"context"
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

// prevCloseChunk: lotes de a mil simbolos para la lectura del prevClose --
// GetPreviousSessionClose es por-simbolo y la query de la subasta es
// barata; el lote solo acota la memoria de la lista.
const prevCloseChunk = 1000

// RefreshPrevClose calcula el prevClose (cierre de la subasta de la sesion
// anterior, ver GetPreviousSessionClose) SOLO para los simbolos cuyo
// prev_close_updated_at quedo fuera de la ventana de mantenimiento actual
// -- el guard por-simbolo con fecha evita recalcular los 13k en cada
// reinicio (corre en el backfill, despues del barrido D1 y antes de H1,
// ver universe_cycle.go). Simbolos sin datos previos se marcan como
// "intentados" (MarkPrevCloseAttempted): no se re-procesan en el mismo
// window, y el proximo window los reintenta igual.
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
	done := 0
	for i := 0; i < len(stale); i += prevCloseChunk {
		end := min(i+prevCloseChunk, len(stale))
		for _, symbol := range stale[i:end] {
			close, err := candles.GetPreviousSessionClose(ctx, symbol, time.Now())
			if err != nil {
				continue
			}
			if close == nil {
				// Sin datos M1 (warrants/OTC sin historia): estampar
				// "intentado" para no re-procesarlo en cada ciclo -- el
				// proximo window lo reintenta igual (el guard compara contra
				// windowStart).
				if err := fundamentals.MarkPrevCloseAttempted(ctx, symbol); err != nil {
					return err
				}
				continue
			}
			if err := fundamentals.UpsertPrevClose(ctx, symbol, *close); err != nil {
				return err
			}
			done++
		}
	}
	log.Info().Int("symbols", done).Dur("elapsed", time.Since(start)).Msg("prev close refresh finished")
	return nil
}
