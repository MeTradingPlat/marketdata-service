package catchup

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

// prevCloseWorkers: RefreshPrevClose era un for secuencial, un round-trip a
// la vez -- confirmado en vivo el 2026-09-01: 9676 simbolos tardaron 34
// MINUTOS (un solo symbol/query cada vez), el paso mas lento por lejos de
// todo el ciclo posterior al rollout en vivo, extendiendo cada reinicio del
// dia con el mismo gate de backfill cerrado de mas. Mismo criterio de
// concurrencia acotada que liveRolloutWorkers (cmd/api/universe_cycle.go) y
// candlesBatchFallbackWorkers (ingestion/get_candles.go) -- pgxpool ya es
// seguro para uso concurrente, no hace falta nada especial del lado de BD.
const prevCloseWorkers = 20

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
	var done, failed atomic.Int64

	jobs := make(chan string, len(stale))
	for _, symbol := range stale {
		jobs <- symbol
	}
	close(jobs)

	var wg sync.WaitGroup
	for i := 0; i < prevCloseWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for symbol := range jobs {
				if refreshOnePrevClose(ctx, candles, fundamentals, symbol) {
					done.Add(1)
				} else {
					failed.Add(1)
				}
			}
		}()
	}
	wg.Wait()

	log.Info().Int64("symbols", done.Load()).Int64("failed", failed.Load()).Dur("elapsed", time.Since(start)).Msg("prev close refresh finished")
	return nil
}

// refreshOnePrevClose devuelve false en cualquier fallo (de lectura o de
// escritura) -- un simbolo fallido no debe frenar a los demas, el proximo
// window lo reintenta igual (mismo criterio que el resto del barrido
// nocturno: mejor un universo parcialmente al dia que ninguno).
func refreshOnePrevClose(ctx context.Context, candles out.CandleRepository, fundamentals out.FundamentalsRepository, symbol string) bool {
	closePrice, err := candles.GetPreviousSessionClose(ctx, symbol, time.Now())
	if err != nil {
		return false
	}
	if closePrice == nil {
		// Sin datos M1 (warrants/OTC sin historia): estampar "intentado"
		// para no re-procesarlo en cada ciclo -- el proximo window lo
		// reintenta igual (el guard compara contra windowStart).
		return fundamentals.MarkPrevCloseAttempted(ctx, symbol) == nil
	}
	return fundamentals.UpsertPrevClose(ctx, symbol, *closePrice) == nil
}
