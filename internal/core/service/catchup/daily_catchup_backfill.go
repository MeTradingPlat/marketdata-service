package catchup

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

const (
	noWatermarkM1Fallback = 24 * time.Hour
	outcomeSaved          = "saved"
	outcomeEmpty          = "empty"
	outcomeErrored        = "errored"
)

func backfillBatchWithRetry(ctx context.Context, gateway out.MarketDataGateway, candles out.CandleRepository, ingest in.IngestCandlesService, batch []job, tf domain.Timeframe, duration time.Duration) ([]string, bool, int) {
	o := backfillBatchWithOutcome(ctx, gateway, candles, ingest, batch, tf, duration)
	return o.empty, o.partial, o.skippedLive
}

func backfillBatchWithOutcome(ctx context.Context, gateway out.MarketDataGateway, candles out.CandleRepository, ingest in.IngestCandlesService, batch []job, tf domain.Timeframe, duration time.Duration) batchOutcome {
	var o batchOutcome
	symbols, skippedSyms := filterBatchNonLiveSymbols(gateway, batch, tf)
	o.skippedLive = len(skippedSyms)
	o.addAccounted(skippedSyms...)
	if len(symbols) == 0 {
		return o
	}
	watermarks, err := candles.GetWatermarksBatch(ctx, symbols, tf)
	if err != nil {
		log.Warn().Err(err).Str("timeframe", string(tf)).Int("symbols", len(symbols)).
			Msg("universe sweep watermark batch failed, falling back to per-symbol backfill")
		for _, sym := range symbols {
			o.recordBackfill(ctx, ingest, job{symbol: sym, tf: tf})
		}
		return o
	}
	froms, upToDate, noWatermark := partitionBatchWatermarks(symbols, watermarks, tf, duration)
	o.upToDate = len(upToDate)
	o.addAccounted(upToDate...)
	reprocessNoWatermark(ctx, ingest, tf, noWatermark, &o)
	if len(froms) > 0 {
		fetchAndSaveBatchOutcome(ctx, gateway, candles, ingest, froms, tf, &o)
	}
	return o
}

func reprocessNoWatermark(ctx context.Context, ingest in.IngestCandlesService, tf domain.Timeframe, syms []string, o *batchOutcome) {
	if len(syms) == 0 {
		return
	}
	o.noWatermark += len(syms)
	sample := syms[:min(len(syms), missingSampleLimit)]
	log.Debug().Str("timeframe", string(tf)).Int("count", len(syms)).Strs("sample", sample).
		Msg("covered symbols without watermark, reprocessing")
	for _, sym := range syms {
		o.recordBackfill(ctx, ingest, job{symbol: sym, tf: tf})
	}
}
