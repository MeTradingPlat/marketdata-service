package catchup

import (
	"context"
	"errors"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

func fetchAndSaveBatchOutcome(ctx context.Context, gateway out.MarketDataGateway, candles out.CandleRepository, ingest in.IngestCandlesService, froms map[string]time.Time, tf domain.Timeframe, o *batchOutcome) {
	result, err := gateway.GetCandlesBatch(ctx, tf, froms)
	if err == nil || errors.Is(err, domain.ErrPartialHistory) {
		withWatermark := err == nil
		if !withWatermark {
			log.Warn().Str("timeframe", string(tf)).Int("symbols", len(froms)).Msg("universe sweep batch cut by timeout, saving without watermark")
			o.partial = true
		}
		empty := emptyBatchSymbols(froms, result)
		o.empty = append(o.empty, empty...)
		o.addAccounted(empty...)
		saveBatchOutcome(ctx, candles, ingest, result, tf, withWatermark, o)
		return
	}
	log.Warn().Err(err).Str("timeframe", string(tf)).Int("symbols", len(froms)).Msg("universe sweep batch fetch failed, falling back to per-symbol backfill")
	for symbol := range froms {
		o.recordBackfill(ctx, ingest, job{symbol: symbol, tf: tf})
	}
}

func saveBatchOutcome(ctx context.Context, candles out.CandleRepository, ingest in.IngestCandlesService, result map[string][]domain.Candle, tf domain.Timeframe, withWatermark bool, o *batchOutcome) {
	merged, bySymbol, emptyClosed := collectClosedBatch(result)
	if len(emptyClosed) > 0 {
		o.empty = append(o.empty, emptyClosed...)
		o.addAccounted(emptyClosed...)
	}
	if len(merged) == 0 {
		return
	}
	if err := candles.Save(ctx, merged, withWatermark); err != nil {
		log.Warn().Err(err).Str("timeframe", string(tf)).Int("symbols", len(bySymbol)).Msg("universe sweep batch save failed, falling back to per-symbol backfill")
		for symbol := range bySymbol {
			o.recordBackfill(ctx, ingest, job{symbol: symbol, tf: tf})
		}
		return
	}
	ingest.RecordTodaysClosedCandles(merged)
	if withWatermark {
		o.saved += len(bySymbol)
	} else {
		o.cut += len(bySymbol)
	}
	for symbol := range bySymbol {
		o.addAccounted(symbol)
	}
}
