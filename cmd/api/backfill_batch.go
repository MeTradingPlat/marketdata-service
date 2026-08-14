package main

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

type backfillJob struct {
	symbol string
	tf     domain.Timeframe
}

// runBackfillBatch es una herramienta de medicion, no un flujo de
// produccion: trae un lote real del universo de TastyTrade, lo rastrea, y
// corre el backfill M1/H1/D1 con un pool de workers -- para saber cuanto
// tarda de verdad antes de decidir cuanto tiempo/recursos dedicarle al
// universo completo.
func runBackfillBatch(ctx context.Context, gateway out.MarketDataGateway, symbolRepo out.SymbolRepository, ingest in.IngestCandlesService, batchSize, workers int) {
	log.Info().Int("batch_size", batchSize).Msg("fetching real TastyTrade symbol universe")
	all, err := gateway.ActiveSymbols(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to fetch active symbols, skipping batch backfill")
		return
	}
	log.Info().Int("universe_size", len(all)).Msg("active symbols fetched")

	if batchSize > len(all) {
		batchSize = len(all)
	}
	batch := all[:batchSize]

	if err := symbolRepo.Upsert(ctx, batch); err != nil {
		log.Error().Err(err).Msg("failed to track batch symbols")
		return
	}

	jobs := make(chan backfillJob, len(batch)*3)
	for _, s := range batch {
		for _, tf := range []domain.Timeframe{domain.D1, domain.H1, domain.M1} {
			jobs <- backfillJob{symbol: s.Symbol, tf: tf}
		}
	}
	close(jobs)

	total := int64(len(batch) * 3)
	var completed int64
	start := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				if err := ingest.Backfill(ctx, j.symbol, j.tf); err != nil {
					log.Error().Err(err).Str("symbol", j.symbol).Str("timeframe", string(j.tf)).Msg("batch backfill job failed")
				}
				if n := atomic.AddInt64(&completed, 1); n%100 == 0 || n == total {
					log.Info().Int64("completed", n).Int64("total", total).Dur("elapsed", time.Since(start)).Msg("batch backfill progress")
				}
			}
		}()
	}
	wg.Wait()

	log.Info().Int("symbols", len(batch)).Int64("requests", total).Dur("elapsed", time.Since(start)).Msg("batch backfill finished")
}
