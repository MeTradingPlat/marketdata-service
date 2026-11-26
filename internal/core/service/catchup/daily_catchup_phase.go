package catchup

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

const (
	backfillMaxAttempts = 3
	backfillRetryDelay  = 5 * time.Second
)

// sweepBatchSize es el tamano de lote del barrido -- el agrupamiento
// original del pool de Java (100 simbolos por suscripcion DxLink): con
// esto la fase D1 pasa de 13k round-trips de add/remove a ~130 mensajes.
const sweepBatchSize = 100

// runPhase corre UN timeframe hasta el final; todos los workers terminan (wg.Wait) antes de la fase siguiente.
func runPhase(ctx context.Context, gateway out.MarketDataGateway, candles out.CandleRepository, ingest in.IngestCandlesService, uncovered, covered []job, tf domain.Timeframe, workers int) {
	start := time.Now()
	empty := runUncoveredPhase(ctx, ingest, uncovered, workers)

	duration, err := tf.Duration()
	if err != nil {
		log.Error().Err(err).Str("timeframe", string(tf)).Msg("universe sweep: resolving timeframe duration")
		return
	}
	coveredEmpty, partialBatches, skippedLive := runCoveredBatches(ctx, gateway, candles, ingest, covered, tf, duration, workers)
	empty = append(empty, coveredEmpty...)

	elapsed := time.Since(start)
	logPhaseFinished(tf, len(uncovered)+len(covered), empty, skippedLive, partialBatches, elapsed)
	reportPhase(tf, len(uncovered)+len(covered), len(empty), partialBatches, elapsed)
}

func runCoveredBatches(ctx context.Context, gateway out.MarketDataGateway, candles out.CandleRepository, ingest in.IngestCandlesService, covered []job, tf domain.Timeframe, duration time.Duration, workers int) ([]string, int, int) {
	batches := make(chan []job, len(covered)/sweepBatchSize+1)
	for i := 0; i < len(covered); i += sweepBatchSize {
		end := min(i+sweepBatchSize, len(covered))
		batches <- covered[i:end]
	}
	close(batches)

	var (
		mu             sync.Mutex
		empty          []string
		partialBatches int
		skippedLive    int
		wg             sync.WaitGroup
	)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for batch := range batches {
				batchEmpty, partial, skipped := backfillBatchWithRetry(ctx, gateway, candles, ingest, batch, tf, duration)
				mergeBatchResult(&mu, &empty, &partialBatches, &skippedLive, batchEmpty, partial, skipped)
			}
		}()
	}
	wg.Wait()
	return empty, partialBatches, skippedLive
}

func logPhaseFinished(tf domain.Timeframe, totalSymbols int, empty []string, skippedLive, partialBatches int, elapsed time.Duration) {
	ev := log.Info()
	if len(empty) > 0 {
		ev = log.Warn()
		sort.Strings(empty)
		sample := empty
		if len(sample) > 50 {
			sample = sample[:50]
		}
		ev = ev.Strs("emptySample", sample)
	}
	ev.Str("timeframe", string(tf)).
		Int("symbols", totalSymbols).
		Int("empty", len(empty)).
		Int("skipped_live", skippedLive).
		Int("partialBatches", partialBatches).
		Dur("elapsed", elapsed).
		Msg("universe sweep phase finished")
}
