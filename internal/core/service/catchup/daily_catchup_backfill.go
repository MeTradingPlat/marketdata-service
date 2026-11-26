package catchup

import (
	"context"
	"errors"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/ingestion"
	"github.com/rs/zerolog/log"
)

// noWatermarkM1Fallback: cuanto pedir hacia atras para un simbolo M1 sin backfill real (solo velas del stream en vivo).
const noWatermarkM1Fallback = 24 * time.Hour

// backfillBatchWithRetry arma el lote: lee el watermark de cada simbolo,
// descarta los que no tienen nada nuevo cerrado todavia (HasNewClosedBar)
// y pide el resto en UNA suscripcion con el margen incremental de cada uno.
// Si el fetch del lote falla, cae al backfill individual con reintentos --
// peor lento que dejar un lote sin datos.
func backfillBatchWithRetry(ctx context.Context, gateway out.MarketDataGateway, candles out.CandleRepository, ingest in.IngestCandlesService, batch []job, tf domain.Timeframe, duration time.Duration) ([]string, bool, int) {
	symbols, skippedLive := filterBatchNonLive(gateway, batch, tf)
	if len(symbols) == 0 {
		return nil, false, skippedLive
	}
	watermarks, err := candles.GetWatermarksBatch(ctx, symbols, tf)
	if err != nil {
		log.Warn().Err(err).Str("timeframe", string(tf)).Int("symbols", len(symbols)).
			Msg("universe sweep watermark batch failed, falling back to per-symbol backfill")
		for _, sym := range symbols {
			backfillWithRetry(ctx, ingest, job{symbol: sym, tf: tf})
		}
		return nil, false, skippedLive
	}
	froms := buildBatchFroms(symbols, watermarks, tf, duration)
	if len(froms) == 0 {
		return nil, false, skippedLive
	}
	return fetchAndSaveBatch(ctx, gateway, candles, ingest, froms, tf, skippedLive)
}

func fetchAndSaveBatch(ctx context.Context, gateway out.MarketDataGateway, candles out.CandleRepository, ingest in.IngestCandlesService, froms map[string]time.Time, tf domain.Timeframe, skippedLive int) ([]string, bool, int) {
	result, err := gateway.GetCandlesBatch(ctx, tf, froms)
	if err == nil {
		empty := emptyBatchSymbols(froms, result)
		saveBatchSweep(ctx, candles, ingest, result, tf, true)
		return empty, false, skippedLive
	}
	if errors.Is(err, domain.ErrPartialHistory) {
		log.Warn().Str("timeframe", string(tf)).Int("symbols", len(froms)).
			Msg("universe sweep batch cut by timeout, saving without watermark")
		empty := emptyBatchSymbols(froms, result)
		saveBatchSweep(ctx, candles, ingest, result, tf, false)
		return empty, true, skippedLive
	}
	log.Warn().Err(err).Str("timeframe", string(tf)).Int("symbols", len(froms)).
		Msg("universe sweep batch fetch failed, falling back to per-symbol backfill")
	for symbol := range froms {
		backfillWithRetry(ctx, ingest, job{symbol: symbol, tf: tf})
	}
	return nil, false, skippedLive
}

// saveBatchSweep junta las velas cerradas de TODOS los simbolos del lote en un solo Save(); si falla, cae al backfill individual.
func saveBatchSweep(ctx context.Context, candles out.CandleRepository, ingest in.IngestCandlesService, result map[string][]domain.Candle, tf domain.Timeframe, withWatermark bool) {
	merged := make([]domain.Candle, 0, len(result))
	bySymbol := make(map[string]struct{}, len(result))
	for symbol, candlesFetched := range result {
		closed := domain.ClosedCandles(candlesFetched, time.Now())
		if len(closed) == 0 {
			continue
		}
		merged = append(merged, closed...)
		bySymbol[symbol] = struct{}{}
	}
	if len(merged) == 0 {
		return
	}
	if err := candles.Save(ctx, merged, withWatermark); err != nil {
		log.Warn().Err(err).Str("timeframe", string(tf)).Int("symbols", len(bySymbol)).
			Msg("universe sweep batch save failed, falling back to per-symbol backfill")
		for symbol := range bySymbol {
			backfillWithRetry(ctx, ingest, job{symbol: symbol, tf: tf})
		}
		return
	}
	// Sin este RecordTodaysClosedCandles, el SnapshotTracker se quedaria con el volumen de ayer hasta el proximo reconcile.
	ingest.RecordTodaysClosedCandles(merged)
}

// backfillWithRetry reintenta un trabajo de backfill fallido; cada worker tiene su propia cola, asi que reintentar solo lo demora a el.
func backfillWithRetry(ctx context.Context, ingest in.IngestCandlesService, j job) bool {
	var err error
	for attempt := 1; attempt <= backfillMaxAttempts; attempt++ {
		if err = ingest.Backfill(ctx, j.symbol, j.tf); err == nil {
			return false
		}
		if attempt < backfillMaxAttempts {
			time.Sleep(backfillRetryDelay)
		}
	}
	if errors.Is(err, ingestion.ErrEmptyBackfill) {
		log.Debug().Str("symbol", j.symbol).Str("timeframe", string(j.tf)).Int("attempts", backfillMaxAttempts).
			Msg("universe sweep job returned no candles after retries")
		return true
	}
	log.Error().Err(err).Str("symbol", j.symbol).Str("timeframe", string(j.tf)).Int("attempts", backfillMaxAttempts).
		Msg("universe sweep job failed after retries")
	return false
}
