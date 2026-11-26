package catchup

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/rs/zerolog/log"
)

func runUncoveredPhaseOutcome(ctx context.Context, ingest in.IngestCandlesService, uncovered []job, workers int) batchOutcome {
	var (
		mu      sync.Mutex
		outcome batchOutcome
		wg      sync.WaitGroup
	)
	queue := make(chan job, len(uncovered))
	for _, j := range uncovered {
		queue <- j
	}
	close(queue)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var local batchOutcome
			for j := range queue {
				local.recordBackfill(ctx, ingest, j)
			}
			mu.Lock()
			outcome.saved += local.saved
			outcome.empty = append(outcome.empty, local.empty...)
			outcome.errored += local.errored
			outcome.accounted = append(outcome.accounted, local.accounted...)
			mu.Unlock()
		}()
	}
	wg.Wait()
	return outcome
}

type phaseSummaryFields struct {
	warn               bool
	emptySample        []string
	includeSkippedLive bool
}

func buildPhaseSummaryFields(tf domain.Timeframe, s *phaseSummary, missingCount int) phaseSummaryFields {
	var emptySample []string
	if len(s.empty) > 0 {
		emptySample = append([]string(nil), s.empty...)
		sort.Strings(emptySample)
		emptySample = emptySample[:min(len(emptySample), 50)]
	}
	return phaseSummaryFields{
		warn:               len(s.empty) > 0 || s.errored > 0 || missingCount > 0,
		emptySample:        emptySample,
		includeSkippedLive: tf == domain.M1 || s.skippedLive > 0,
	}
}

func logPhaseSummary(tf domain.Timeframe, s *phaseSummary, missingCount int, missingSample []string, elapsed time.Duration) {
	fields := buildPhaseSummaryFields(tf, s, missingCount)
	ev := log.Info()
	if fields.warn {
		ev = log.Warn()
	}
	if len(fields.emptySample) > 0 {
		ev = ev.Strs("emptySample", fields.emptySample)
	}
	if missingCount > 0 {
		ev = ev.Strs("missingSample", missingSample)
	}
	if fields.includeSkippedLive {
		ev = ev.Int("skipped_live", s.skippedLive)
	}
	ev.Str("timeframe", string(tf)).
		Int("requested", s.requested).Int("saved", s.saved).Int("empty", len(s.empty)).
		Int("cut", s.cut).Int("up_to_date", s.upToDate).Int("errored", s.errored).
		Int("no_watermark", s.noWatermark).Int("missing", missingCount).
		Int("partialBatches", s.partialBatches).Dur("elapsed", elapsed).
		Msg("universe sweep phase finished")
}
