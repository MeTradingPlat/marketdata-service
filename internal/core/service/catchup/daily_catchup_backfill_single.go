package catchup

import (
	"context"
	"errors"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/ingestion"
	"github.com/rs/zerolog/log"
)

func backfillOutcome(ctx context.Context, ingest in.IngestCandlesService, j job) string {
	var err error
	for attempt := 1; attempt <= backfillMaxAttempts; attempt++ {
		if err = ingest.Backfill(ctx, j.symbol, j.tf); err == nil {
			return outcomeSaved
		}
		if attempt < backfillMaxAttempts {
			time.Sleep(backfillRetryDelay)
		}
	}
	if errors.Is(err, ingestion.ErrEmptyBackfill) {
		log.Debug().Str("symbol", j.symbol).Str("timeframe", string(j.tf)).Int("attempts", backfillMaxAttempts).
			Msg("universe sweep job returned no candles after retries")
		return outcomeEmpty
	}
	log.Error().Err(err).Str("symbol", j.symbol).Str("timeframe", string(j.tf)).Int("attempts", backfillMaxAttempts).
		Msg("universe sweep job failed after retries")
	return outcomeErrored
}
