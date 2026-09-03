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

// prevCloseWorkers paraleliza solo la ESCRITURA (Upsert/MarkAttempted, una
// fila por simbolo por su propia clave) -- la LECTURA ya no es por-simbolo:
// GetPreviousSessionCloseBatch trae el universo entero en una sola consulta
// (o unas pocas, una por dia atras si algun simbolo no opero ayer). Antes
// RefreshPrevClose llamaba GetPreviousSessionClose (10 ventanas OR-eadas)
// UNA VEZ POR SIMBOLO -- confirmado en vivo el 2026-09-01/02: 9676 simbolos
// asi tardaron 34+ minutos (y bajo presion de Postgres, con
// statement_timeout de por medio, mucho mas), el paso mas lento por lejos
// de todo el ciclo, pese a que la respuesta real es "una consulta a todo el
// universo pidiendo simbolo+close" -- exactamente lo que ya hacia la
// version batch para OTRO llamador (GetSnapshot) sin que este la usara.
const prevCloseWorkers = 20

// RefreshPrevClose calcula el prevClose (cierre de la subasta de la sesion
// anterior) SOLO para los simbolos cuyo prev_close_updated_at quedo fuera
// de la ventana de mantenimiento actual -- el guard por-simbolo con fecha
// evita recalcular los 13k en cada reinicio (corre en el backfill, despues
// del barrido D1 y antes de H1, ver universe_cycle.go). Simbolos sin datos
// previos se marcan como "intentados" (MarkPrevCloseAttempted): no se
// re-procesan en el mismo window, y el proximo window los reintenta igual.
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
				if writeOnePrevClose(ctx, fundamentals, symbol, closes) {
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

// writeOnePrevClose devuelve false en cualquier fallo de escritura -- un
// simbolo fallido no debe frenar a los demas, el proximo window lo
// reintenta igual (mismo criterio que el resto del barrido nocturno: mejor
// un universo parcialmente al dia que ninguno).
func writeOnePrevClose(ctx context.Context, fundamentals out.FundamentalsRepository, symbol string, closes map[string]float64) bool {
	closePrice, ok := closes[symbol]
	if !ok {
		// Sin datos M1 (warrants/OTC sin historia): estampar "intentado"
		// para no re-procesarlo en cada ciclo -- el proximo window lo
		// reintenta igual (el guard compara contra windowStart).
		return fundamentals.MarkPrevCloseAttempted(ctx, symbol) == nil
	}
	return fundamentals.UpsertPrevClose(ctx, symbol, closePrice) == nil
}
