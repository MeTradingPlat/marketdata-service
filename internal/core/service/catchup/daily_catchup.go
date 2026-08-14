package catchup

import (
	"context"
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

const (
	postMidnightDelay = 5 * time.Minute
	catchUpWorkers    = 20
)

// StartDailyCatchUp reemplaza la agregacion SQL M1->H1/D1 que se tenia
// antes: en vez de sintetizar H1/D1 nosotros mismos, le pide a TastyTrade
// las barras nativas que falten -- mas simple, mas fiel (son las barras
// reales, no una reconstruccion), y sin la clase de deadlock que tenia la
// agregacion (esa competia con Save() por la misma fila de watermarks;
// aqui cada simbolo es su propio Backfill(), ya idempotente via UPSERT).
// Corre una vez al arrancar (por si el servicio estuvo caido) y luego una
// vez al dia, poco despues de medianoche UTC -- Backfill() ya es
// incremental, asi que este barrido diario es barato: solo trae lo que
// cerro desde el ultimo watermark de cada simbolo.
func StartDailyCatchUp(ctx context.Context, symbols out.SymbolRepository, ingest in.IngestCandlesService) {
	go func() {
		runOnce(ctx, symbols, ingest)
		for {
			wait := time.Until(nextRunAt(time.Now().UTC()))
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
				runOnce(ctx, symbols, ingest)
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

func runOnce(ctx context.Context, symbols out.SymbolRepository, ingest in.IngestCandlesService) {
	tracked, err := symbols.Tracked(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to list tracked symbols for daily catch-up")
		return
	}

	type job struct {
		symbol string
		tf     domain.Timeframe
	}
	jobs := make(chan job, len(tracked)*2)
	for _, s := range tracked {
		jobs <- job{symbol: s.Symbol, tf: domain.D1}
		jobs <- job{symbol: s.Symbol, tf: domain.H1}
		// M1 de un simbolo con streaming en vivo activo es un no-op seguro
		// (ver hasLiveSub en FetchHistory) -- para el resto del universo, le
		// da a M1 el mismo refresco incremental diario que ya tienen D1/H1.
		jobs <- job{symbol: s.Symbol, tf: domain.M1}
	}
	close(jobs)

	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < catchUpWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				if err := ingest.Backfill(ctx, j.symbol, j.tf); err != nil {
					log.Error().Err(err).Str("symbol", j.symbol).Str("timeframe", string(j.tf)).
						Msg("daily catch-up job failed")
				}
			}
		}()
	}
	wg.Wait()

	log.Info().Int("symbols", len(tracked)).Dur("elapsed", time.Since(start)).Msg("daily catch-up finished")
}
