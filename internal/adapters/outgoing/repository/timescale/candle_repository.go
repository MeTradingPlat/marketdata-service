package timescale

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CandleRepository struct {
	pool *pgxpool.Pool
}

func NewCandleRepository(pool *pgxpool.Pool) *CandleRepository {
	return &CandleRepository{pool: pool}
}

const deadlockRetries = 5

// execWithDeadlockRetry reintenta ante un deadlock de Postgres (40P01) --
// mucho menos probable ahora que ya no hay una transaccion grande
// multi-simbolo compitiendo con Save() (ver daily_catchup.go), pero dos
// escrituras concurrentes para el MISMO simbolo (ej. catch-up diario y
// stream en vivo tocando el mismo D1 a la vez) todavia podrian chocar --
// reintentar es seguro porque todo es UPSERT.
func execWithDeadlockRetry(ctx context.Context, exec func(context.Context) error) error {
	var err error
	for attempt := 0; attempt < deadlockRetries; attempt++ {
		err = exec(ctx)
		var pgErr *pgconn.PgError
		if err == nil || !errors.As(err, &pgErr) || pgErr.Code != "40P01" {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
	}
	return err
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

// Solo M1 deja watermark aqui -- H1/D1 nativas de TastyTrade no lo
// necesitan (su frescura ya se puede leer de MAX(ts) en candles via
// GetWatermark), y escribirlo tambien para ellas competia por la MISMA fila
// watermarks(symbol_id, 'D1'/'H1') que el job de agregacion, con origenes
// distintos (nativo vs derivado de M1) mezclados en una sola fila sin
// distincion. Esa doble escritura era justamente lo que producia el
// deadlock confirmado en vivo entre Save() y AggregateH1/D1 (ambos
// intentando actualizar la misma fila al mismo tiempo) -- separar los
// caminos elimina la colision de raiz en vez de solo reintentarla.
func (r *CandleRepository) Save(ctx context.Context, candles []domain.Candle) error {
	if len(candles) == 0 {
		return nil
	}
	return execWithDeadlockRetry(ctx, func(ctx context.Context) error {
		batch := &pgx.Batch{}
		for _, c := range candles {
			batch.Queue(upsertCandleSQL, c.Symbol, string(c.Timeframe), c.Timestamp,
				c.Open, c.High, c.Low, c.Close, c.Volume, c.TradeCount, c.VWAP, c.Source)
			if c.Timeframe == domain.M1 {
				batch.Queue(upsertWatermarkSQL, c.Symbol, string(c.Timeframe), c.Timestamp)
			}
		}
		results := r.pool.SendBatch(ctx, batch)
		defer results.Close()
		for _, c := range candles {
			if _, err := results.Exec(); err != nil {
				return fmt.Errorf("upserting candle batch: %w", err)
			}
			if c.Timeframe == domain.M1 {
				if _, err := results.Exec(); err != nil {
					return fmt.Errorf("upserting watermark batch: %w", err)
				}
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

const symbolsWithDataSQL = `
	SELECT DISTINCT s.symbol
	FROM candles c JOIN tracked_symbols s ON s.symbol_id = c.symbol_id
	WHERE c.timeframe = $1
`

// SymbolsWithData es el equivalente en lote de "tiene watermark" -- una
// consulta para el universo entero en vez de una por simbolo (13k consultas
// individuales para lo mismo), pensada para priorizar una cola de backfill:
// separar de una vez los simbolos que nunca se tocaron de los que ya tienen
// datos, sin pagar el costo de GetWatermark simbolo por simbolo.
func (r *CandleRepository) SymbolsWithData(ctx context.Context, timeframe domain.Timeframe) (map[string]struct{}, error) {
	rows, err := r.pool.Query(ctx, symbolsWithDataSQL, string(timeframe))
	if err != nil {
		return nil, fmt.Errorf("querying symbols with %s data: %w", timeframe, err)
	}
	defer rows.Close()

	symbols := make(map[string]struct{})
	for rows.Next() {
		var symbol string
		if err := rows.Scan(&symbol); err != nil {
			return nil, fmt.Errorf("scanning symbol row: %w", err)
		}
		symbols[symbol] = struct{}{}
	}
	return symbols, rows.Err()
}
