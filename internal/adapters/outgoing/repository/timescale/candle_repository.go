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
	// writePool es un pool chico DEDICADO a Save() para que el streaming M1 no compita por slots con las lecturas de charts.
	writePool *pgxpool.Pool
	// snapshotPool es un pool chico DEDICADO a fundamentals/realtime en
	// lote (GetIntradaySessionsBatch/GetPreviousSessionCloseBatch) -- ver
	// storage.snapshotPoolConns: aisla esas dos consultas de las
	// agregaciones time_bucket de H1/D1 que compiten por el pool general.
	snapshotPool *pgxpool.Pool
}

func NewCandleRepository(pool, writePool, snapshotPool *pgxpool.Pool) *CandleRepository {
	return &CandleRepository{pool: pool, writePool: writePool, snapshotPool: snapshotPool}
}

const deadlockRetries = 5

// execWithDeadlockRetry reintenta ante un deadlock de Postgres (40P01); reintentar es seguro porque todo es UPSERT.
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

// verified marca si la vela vino del refill/backfill (watermark TRUE, dato
// verificado de TastyTrade) o del live en vivo (provisional -- un reinicio
// la re-pide). El upsert con verified=TRUE no pisa una verified=FALSE con
// un valor mas viejo de la misma barra; y viceversa, el live jamas pisa una
// verificada con su provisional.
const upsertCandleSQL = `
	INSERT INTO %[1]s (symbol_id, timeframe, ts, open, high, low, close, volume, trade_count, vwap, source, verified, last_written_at)
	SELECT symbol_id, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, now() FROM tracked_symbols WHERE symbol = $1
	ON CONFLICT (symbol_id, timeframe, ts) DO UPDATE SET
		open = EXCLUDED.open, high = EXCLUDED.high, low = EXCLUDED.low, close = EXCLUDED.close,
		volume = EXCLUDED.volume, trade_count = EXCLUDED.trade_count, vwap = EXCLUDED.vwap,
		verified = %[1]s.verified OR EXCLUDED.verified, last_written_at = now()
	WHERE EXCLUDED.verified OR NOT %[1]s.verified
`

var (
	upsertCandleSQLCandles        = fmt.Sprintf(upsertCandleSQL, "candles")
	upsertCandleSQLCandlesHistory = fmt.Sprintf(upsertCandleSQL, "candles_history")
)

const upsertWatermarkSQL = `
	INSERT INTO watermarks (symbol_id, timeframe, last_ts, updated_at)
	SELECT symbol_id, $2, $3, now() FROM tracked_symbols WHERE symbol = $1
	ON CONFLICT (symbol_id, timeframe) DO UPDATE SET
		last_ts = GREATEST(watermarks.last_ts, EXCLUDED.last_ts), updated_at = now()
`

// updateLastVolumeSQL solo se encola para velas D1 -- la comparacion contra
// last_volume_ts evita que el re-fetch defensivo del catch-up nocturno
// (10 dias hacia atras, ver ingestion.IncrementalMargin) pise el volumen mas
// reciente con uno mas viejo si llega despues en el mismo batch.
const updateLastVolumeSQL = `
	UPDATE tracked_symbols SET last_volume = $2, last_volume_ts = $3
	WHERE symbol = $1 AND (last_volume_ts IS NULL OR last_volume_ts < $3)
`

// Todo timeframe deja watermark aqui; watermarks es una tabla chica que se lee por clave primaria sin tocar la hypertable.
func (r *CandleRepository) Save(ctx context.Context, candles []domain.Candle, withWatermark bool) error {
	if len(candles) == 0 {
		return nil
	}
	latestDailies := latestDailyBySymbol(candles)
	return execWithDeadlockRetry(ctx, func(ctx context.Context) error {
		batch := &pgx.Batch{}
		for _, c := range candles {
			query := upsertCandleSQLCandles
			if candleTable(c.Timeframe) == "candles_history" {
				query = upsertCandleSQLCandlesHistory
			}
			batch.Queue(query, c.Symbol, string(c.Timeframe), c.Timestamp,
				c.Open, c.High, c.Low, c.Close, c.Volume, c.TradeCount, c.VWAP, c.Source, withWatermark)
			if withWatermark {
				batch.Queue(upsertWatermarkSQL, c.Symbol, string(c.Timeframe), c.Timestamp)
			}
		}
		for _, c := range latestDailies {
			batch.Queue(updateLastVolumeSQL, c.Symbol, c.Volume, c.Timestamp)
		}
		results := r.writePool.SendBatch(ctx, batch)
		defer results.Close()
		var affected int64
		for range candles {
			tag, err := results.Exec()
			if err != nil {
				return fmt.Errorf("upserting candle batch: %w", err)
			}
			affected += tag.RowsAffected()
			if withWatermark {
				if _, err := results.Exec(); err != nil {
					return fmt.Errorf("upserting watermark batch: %w", err)
				}
			}
		}
		checkBatchRowsAffected(int64(len(candles)), affected, withWatermark)
		for range latestDailies {
			if _, err := results.Exec(); err != nil {
				return fmt.Errorf("updating last volume: %w", err)
			}
		}
		return nil
	})
}
