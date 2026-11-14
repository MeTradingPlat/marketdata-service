package timescale

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

const upsertPrevCloseSQL = `
	INSERT INTO dividends (symbol_id, prev_close, prev_close_updated_at)
	SELECT symbol_id, $2, now() FROM tracked_symbols WHERE symbol = $1
	ON CONFLICT (symbol_id) DO UPDATE SET prev_close = EXCLUDED.prev_close, prev_close_updated_at = now()
`

const markPrevCloseAttemptedSQL = `
	INSERT INTO dividends (symbol_id, prev_close_updated_at)
	SELECT symbol_id, now() FROM tracked_symbols WHERE symbol = $1
	ON CONFLICT (symbol_id) DO UPDATE SET prev_close_updated_at = now()
`

// UpsertPrevCloseBatch guarda el prevClose calculado en el backfill (para
// los simbolos que tuvieron uno) y estampa prev_close_updated_at sin tocar
// prev_close para los que no (mismo criterio "corrio y no encontro dato"
// distinto de "nunca corrio" que ya tenia MarkPrevCloseAttempted -- sin
// eso, los simbolos sin M1 quedaban en la lista stale para siempre).
// UN SOLO pgx.Batch para todo el lote -- antes cada simbolo pagaba su
// propio round trip via un pool de workers (confirmado en vivo el
// 2026-09-04: contribuyo a saturar Postgres en la apertura de mercado
// junto con el resto de consultas per-simbolo de ese incidente).
func (r *FundamentalsRepository) UpsertPrevCloseBatch(ctx context.Context, closes map[string]float64, attemptedOnly []string) error {
	order := lockOrder(closes, attemptedOnly)
	if len(order) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, symbol := range order {
		if close, ok := closes[symbol]; ok {
			batch.Queue(upsertPrevCloseSQL, symbol, close)
		} else {
			batch.Queue(markPrevCloseAttemptedSQL, symbol)
		}
	}
	results := r.pool.SendBatch(ctx, batch)
	defer results.Close()
	for range order {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("upserting prev close batch: %w", err)
		}
	}
	return nil
}

// GetSymbolsWithStalePrevPostMarketVolume: mismo guard por-simbolo que
// GetSymbolsWithStalePrevClose, columna prev_post_market_volume_updated_at.
func (r *FundamentalsRepository) GetSymbolsWithStalePrevPostMarketVolume(ctx context.Context, windowStart time.Time) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT s.symbol
		FROM tracked_symbols s
		LEFT JOIN dividends d ON d.symbol_id = s.symbol_id
		WHERE s.is_active = TRUE
		  AND (d.prev_post_market_volume_updated_at IS NULL OR d.prev_post_market_volume_updated_at < $1)
		ORDER BY s.symbol`, windowStart)
	if err != nil {
		return nil, fmt.Errorf("listing symbols with stale prev post market volume: %w", err)
	}
	defer rows.Close()
	var symbols []string
	for rows.Next() {
		var symbol string
		if err := rows.Scan(&symbol); err != nil {
			return nil, fmt.Errorf("scanning stale prev post market volume symbol: %w", err)
		}
		symbols = append(symbols, symbol)
	}
	return symbols, rows.Err()
}

const upsertPrevPostMarketVolumeSQL = `
	INSERT INTO dividends (symbol_id, prev_post_market_volume, prev_post_market_volume_updated_at)
	SELECT symbol_id, $2, now() FROM tracked_symbols WHERE symbol = $1
	ON CONFLICT (symbol_id) DO UPDATE SET prev_post_market_volume = EXCLUDED.prev_post_market_volume, prev_post_market_volume_updated_at = now()
`

const markPrevPostMarketVolumeAttemptedSQL = `
	INSERT INTO dividends (symbol_id, prev_post_market_volume_updated_at)
	SELECT symbol_id, now() FROM tracked_symbols WHERE symbol = $1
	ON CONFLICT (symbol_id) DO UPDATE SET prev_post_market_volume_updated_at = now()
`

// UpsertPrevPostMarketVolumeBatch: mismo patron que UpsertPrevCloseBatch --
// UN SOLO pgx.Batch para todo el lote de simbolos con dato y los que se
// marcan como intentados sin dato, ver domain.Fundamentals.
// PrevPostMarketVolume.
func (r *FundamentalsRepository) UpsertPrevPostMarketVolumeBatch(ctx context.Context, volumes map[string]int64, attemptedOnly []string) error {
	order := lockOrder(volumes, attemptedOnly)
	if len(order) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, symbol := range order {
		if volume, ok := volumes[symbol]; ok {
			batch.Queue(upsertPrevPostMarketVolumeSQL, symbol, volume)
		} else {
			batch.Queue(markPrevPostMarketVolumeAttemptedSQL, symbol)
		}
	}
	results := r.pool.SendBatch(ctx, batch)
	defer results.Close()
	for range order {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("upserting prev post market volume batch: %w", err)
		}
	}
	return nil
}
