package catchup

import (
	"context"
	"fmt"
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
func StartDailyCatchUp(ctx context.Context, gateway out.MarketDataGateway, symbols out.SymbolRepository, candles out.CandleRepository, ingest in.IngestCandlesService) {
	go func() {
		runOnce(ctx, gateway, symbols, candles, ingest)
		for {
			wait := time.Until(nextRunAt(time.Now().UTC()))
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
				runOnce(ctx, gateway, symbols, candles, ingest)
			}
		}
	}()
}

// reconcileUniverse trae el universo activo real de TastyTrade antes de
// cada corrida -- un simbolo nuevo (IPO, resplit, etc.) entra a Tracked()
// de inmediato en vez de esperar a que alguien lo agregue a mano, y uno que
// ya no figura como activo se desactiva (no se borra ninguna vela ya
// guardada, solo deja de pedirsele mas historia). Si el fetch falla o
// vuelve vacio (fallo silencioso del lado de TastyTrade), no se toca nada
// -- mejor un universo desactualizado que desactivar todo por un fetch roto.
func reconcileUniverse(ctx context.Context, gateway out.MarketDataGateway, symbols out.SymbolRepository) error {
	active, err := gateway.ActiveSymbols(ctx)
	if err != nil {
		return fmt.Errorf("fetching active symbol universe: %w", err)
	}
	if len(active) == 0 {
		return fmt.Errorf("active symbol universe came back empty, skipping reconciliation")
	}
	if err := symbols.Upsert(ctx, active); err != nil {
		return fmt.Errorf("upserting active symbols: %w", err)
	}

	tracked, err := symbols.Tracked(ctx)
	if err != nil {
		return fmt.Errorf("listing tracked symbols: %w", err)
	}
	activeSet := make(map[string]struct{}, len(active))
	for _, s := range active {
		activeSet[s.Symbol] = struct{}{}
	}
	var stale []string
	for _, s := range tracked {
		if _, ok := activeSet[s.Symbol]; !ok {
			stale = append(stale, s.Symbol)
		}
	}
	if len(stale) == 0 {
		return nil
	}
	log.Info().Int("count", len(stale)).Msg("deactivating symbols no longer in the active universe")
	return symbols.Deactivate(ctx, stale)
}

type job struct {
	symbol string
	tf     domain.Timeframe
}

// prioritizedJobs pone primero los simbolos que TODAVIA no tienen ninguna
// vela D1/H1 guardada -- sin esto, una corrida sobre el universo entero
// gasta su tiempo re-chequeando simbolos que ya estan al dia (un chequeo
// con al menos una barra nueva cuesta lo mismo en red que traer la
// profundidad completa de un simbolo nuevo, no es proporcional a cuantas
// barras trae) antes de llegar siquiera a los que nunca se tocaron.
// SymbolsWithData es una consulta en lote (2 en total), no una por simbolo.
func prioritizedJobs(ctx context.Context, candles out.CandleRepository, tracked []domain.Symbol) chan job {
	withD1, err := candles.SymbolsWithData(ctx, domain.D1)
	if err != nil {
		log.Error().Err(err).Msg("failed to check which symbols already have D1 data, treating all as uncovered")
		withD1 = map[string]struct{}{}
	}
	withH1, err := candles.SymbolsWithData(ctx, domain.H1)
	if err != nil {
		log.Error().Err(err).Msg("failed to check which symbols already have H1 data, treating all as uncovered")
		withH1 = map[string]struct{}{}
	}

	var uncovered, covered []job
	for _, s := range tracked {
		if _, ok := withD1[s.Symbol]; ok {
			covered = append(covered, job{symbol: s.Symbol, tf: domain.D1})
		} else {
			uncovered = append(uncovered, job{symbol: s.Symbol, tf: domain.D1})
		}
		if _, ok := withH1[s.Symbol]; ok {
			covered = append(covered, job{symbol: s.Symbol, tf: domain.H1})
		} else {
			uncovered = append(uncovered, job{symbol: s.Symbol, tf: domain.H1})
		}
	}

	jobs := make(chan job, len(uncovered)+len(covered))
	for _, j := range uncovered {
		jobs <- j
	}
	for _, j := range covered {
		jobs <- j
	}
	close(jobs)
	return jobs
}

func nextRunAt(now time.Time) time.Time {
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	next := midnight.Add(postMidnightDelay)
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

func runOnce(ctx context.Context, gateway out.MarketDataGateway, symbols out.SymbolRepository, candles out.CandleRepository, ingest in.IngestCandlesService) {
	if err := reconcileUniverse(ctx, gateway, symbols); err != nil {
		log.Error().Err(err).Msg("failed to reconcile symbol universe, backfilling last known tracked list")
	}

	tracked, err := symbols.Tracked(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to list tracked symbols for daily catch-up")
		return
	}

	jobs := prioritizedJobs(ctx, candles, tracked)

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
