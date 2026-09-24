package timescale

import (
	"context"
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/jackc/pgx/v5"
)

const upsertPreMarketEndSQL = `
	INSERT INTO day_volumes (symbol_id, day, volume, pre_market_end_volume, updated_at)
	SELECT symbol_id, $2, $3, $3, now() FROM tracked_symbols WHERE symbol = $1
	ON CONFLICT (symbol_id) DO UPDATE
	SET pre_market_end_volume = EXCLUDED.pre_market_end_volume,
		regular_end_volume = CASE WHEN day_volumes.day = EXCLUDED.day THEN day_volumes.regular_end_volume END,
		volume = CASE WHEN day_volumes.day = EXCLUDED.day THEN day_volumes.volume ELSE EXCLUDED.volume END,
		day = EXCLUDED.day, updated_at = now()`

const upsertRegularEndSQL = `
	INSERT INTO day_volumes (symbol_id, day, volume, regular_end_volume, updated_at)
	SELECT symbol_id, $2, $3, $3, now() FROM tracked_symbols WHERE symbol = $1
	ON CONFLICT (symbol_id) DO UPDATE
	SET regular_end_volume = EXCLUDED.regular_end_volume,
		pre_market_end_volume = CASE WHEN day_volumes.day = EXCLUDED.day THEN day_volumes.pre_market_end_volume END,
		volume = CASE WHEN day_volumes.day = EXCLUDED.day THEN day_volumes.volume ELSE EXCLUDED.volume END,
		day = EXCLUDED.day, updated_at = now()`

func (r *DayVolumeRepository) SavePreMarketEnd(ctx context.Context, day time.Time, volumes map[string]int64) error {
	return r.saveBoundary(ctx, upsertPreMarketEndSQL, day, volumes)
}

func (r *DayVolumeRepository) SaveRegularEnd(ctx context.Context, day time.Time, volumes map[string]int64) error {
	return r.saveBoundary(ctx, upsertRegularEndSQL, day, volumes)
}

func (r *DayVolumeRepository) saveBoundary(ctx context.Context, sql string, day time.Time, volumes map[string]int64) error {
	if len(volumes) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for symbol, volume := range volumes {
		batch.Queue(sql, symbol, day, volume)
	}
	results := r.pool.SendBatch(ctx, batch)
	defer results.Close()
	for range volumes {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("upserting day volume boundary batch: %w", err)
		}
	}
	return nil
}

const getBoundariesSQL = `
	SELECT s.symbol, v.pre_market_end_volume, v.regular_end_volume
	FROM day_volumes v JOIN tracked_symbols s ON s.symbol_id = v.symbol_id
	WHERE s.symbol = ANY($1) AND v.day = $2
		AND (v.pre_market_end_volume IS NOT NULL OR v.regular_end_volume IS NOT NULL)`

func (r *DayVolumeRepository) GetBoundaries(ctx context.Context, symbols []string, day time.Time) (out.DayBoundaryVolumes, error) {
	result := out.DayBoundaryVolumes{PreMarketEnd: map[string]int64{}, RegularEnd: map[string]int64{}}
	rows, err := r.pool.Query(ctx, getBoundariesSQL, symbols, day)
	if err != nil {
		return result, fmt.Errorf("loading day volume boundaries for %d symbols: %w", len(symbols), err)
	}
	defer rows.Close()
	for rows.Next() {
		var symbol string
		var preMarketEnd, regularEnd *int64
		if err := rows.Scan(&symbol, &preMarketEnd, &regularEnd); err != nil {
			return result, fmt.Errorf("scanning day volume boundary: %w", err)
		}
		if preMarketEnd != nil {
			result.PreMarketEnd[symbol] = *preMarketEnd
		}
		if regularEnd != nil {
			result.RegularEnd[symbol] = *regularEnd
		}
	}
	return result, rows.Err()
}
