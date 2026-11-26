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
	summary := newPhaseSummary(uncovered, covered)
	summary.merge(runUncoveredPhaseOutcome(ctx, ingest, uncovered, workers))

	duration, err := tf.Duration()
	if err != nil {
		log.Error().Err(err).Str("timeframe", string(tf)).Msg("universe sweep: resolving timeframe duration")
		return
	}
	runCoveredBatchesSummary(ctx, gateway, candles, ingest, covered, tf, duration, workers, summary)

	elapsed := time.Since(start)
	missingCount, missingSample := summary.missing(uncovered, covered)
	logPhaseSummary(tf, summary, missingCount, missingSample, elapsed)
	reportPhase(tf, summary.requested, len(summary.empty), summary.partialBatches, elapsed)
}

func runCoveredBatchesSummary(ctx context.Context, gateway out.MarketDataGateway, candles out.CandleRepository, ingest in.IngestCandlesService, covered []job, tf domain.Timeframe, duration time.Duration, workers int, summary *phaseSummary) {
	batches := make(chan []job, len(covered)/sweepBatchSize+1)
	for i := 0; i < len(covered); i += sweepBatchSize {
		batches <- covered[i:min(i+sweepBatchSize, len(covered))]
	}
	close(batches)

	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for batch := range batches {
				o := backfillBatchWithOutcome(ctx, gateway, candles, ingest, batch, tf, duration)
				mu.Lock()
				summary.merge(o)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
}
