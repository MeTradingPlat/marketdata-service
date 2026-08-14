package timescale

import (
	"context"
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CandleRepository struct {
	pool *pgxpool.Pool
}

func NewCandleRepository(pool *pgxpool.Pool) *CandleRepository {
	return &CandleRepository{pool: pool}
}

const upsertCandleSQL = `
	INSERT INTO candles (symbol_id, timeframe, ts, open, high, low, close, volume, trade_count, vwap, source)
	SELECT symbol_id, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11 FROM tracked_symbols WHERE symbol = $1
	ON CONFLICT (symbol_id, timeframe, ts) DO UPDATE SET
		open = EXCLUDED.open, high = EXCLUDED.high, low = EXCLUDED.low, close = EXCLUDED.close,
		volume = EXCLUDED.volume, trade_count = EXCLUDED.trade_count, vwap = EXCLUDED.vwap
`

const upsertWatermarkSQL = `
	INSERT INTO watermarks (symbol_id, timeframe, last_ts, updated_at)
	SELECT symbol_id, $2, $3, now() FROM tracked_symbols WHERE symbol = $1
	ON CONFLICT (symbol_id, timeframe) DO UPDATE SET
		last_ts = GREATEST(watermarks.last_ts, EXCLUDED.last_ts), updated_at = now()
`

// Cada vela guardada (backfill o M1 en vivo) deja su propio watermark en el
// mismo batch -- una sola llamada a Save() cubre ambos, sin un camino
// separado solo para el streaming en vivo. Reintenta ante deadlock (mismo
// motivo que AggregateH1/D1: candles y watermarks comparten FK hacia
// tracked_symbols, y con muchos workers de backfill concurrentes mas el job
// de agregacion, Postgres puede detectar un ciclo de locks entre sesiones
// -- confirmado en vivo, es idempotente reintentar gracias a ON CONFLICT).
func (r *CandleRepository) Save(ctx context.Context, candles []domain.Candle) error {
	if len(candles) == 0 {
		return nil
	}
	return execWithDeadlockRetry(ctx, func(ctx context.Context) error {
		batch := &pgx.Batch{}
		for _, c := range candles {
			batch.Queue(upsertCandleSQL, c.Symbol, string(c.Timeframe), c.Timestamp,
				c.Open, c.High, c.Low, c.Close, c.Volume, c.TradeCount, c.VWAP, c.Source)
			batch.Queue(upsertWatermarkSQL, c.Symbol, string(c.Timeframe), c.Timestamp)
		}
		results := r.pool.SendBatch(ctx, batch)
		defer results.Close()
		for range candles {
			if _, err := results.Exec(); err != nil {
				return fmt.Errorf("upserting candle batch: %w", err)
			}
			if _, err := results.Exec(); err != nil {
				return fmt.Errorf("upserting watermark batch: %w", err)
			}
		}
		return nil
	})
}

const getCandlesSQL = `
	SELECT ts, open, high, low, close, volume, trade_count, vwap, source FROM (
		SELECT c.ts, c.open, c.high, c.low, c.close, c.volume, c.trade_count, c.vwap, c.source
		FROM candles c JOIN tracked_symbols s ON s.symbol_id = c.symbol_id
		WHERE s.symbol = $1 AND c.timeframe = $2
		ORDER BY c.ts DESC LIMIT $3
	) recent ORDER BY ts ASC
`

func (r *CandleRepository) GetCandles(ctx context.Context, symbol string, timeframe domain.Timeframe, bars int) ([]domain.Candle, error) {
	rows, err := r.pool.Query(ctx, getCandlesSQL, symbol, string(timeframe), bars)
	if err != nil {
		return nil, fmt.Errorf("querying candles for %s %s: %w", symbol, timeframe, err)
	}
	defer rows.Close()

	var candles []domain.Candle
	for rows.Next() {
		c := domain.Candle{Symbol: symbol, Timeframe: timeframe}
		if err := rows.Scan(&c.Timestamp, &c.Open, &c.High, &c.Low, &c.Close, &c.Volume, &c.TradeCount, &c.VWAP, &c.Source); err != nil {
			return nil, fmt.Errorf("scanning candle row: %w", err)
		}
		candles = append(candles, c)
	}
	return candles, rows.Err()
}

const watermarkSQL = `
	SELECT MAX(c.ts), MIN(c.ts)
	FROM candles c JOIN tracked_symbols s ON s.symbol_id = c.symbol_id
	WHERE s.symbol = $1 AND c.timeframe = $2
`

func (r *CandleRepository) GetWatermark(ctx context.Context, symbol string, timeframe domain.Timeframe) (newest, oldest *time.Time, err error) {
	if err := r.pool.QueryRow(ctx, watermarkSQL, symbol, string(timeframe)).Scan(&newest, &oldest); err != nil {
		return nil, nil, fmt.Errorf("querying watermark for %s %s: %w", symbol, timeframe, err)
	}
	return newest, oldest, nil
}
