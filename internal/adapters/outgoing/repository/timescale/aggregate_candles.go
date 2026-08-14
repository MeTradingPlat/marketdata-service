package timescale

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

const deadlockRetries = 3

// execWithDeadlockRetry reintenta ante un deadlock de Postgres (40P01) --
// esperado bajo escritura concurrente pesada (confirmado en vivo: choco con
// un backfill grande arrancando al mismo tiempo que el catch-up de
// agregacion al reiniciar), y por diseño transitorio: reintentar casi
// siempre basta porque Postgres ya aborto una de las dos transacciones en
// conflicto.
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

// El watermark es el ts mas reciente ya agregado (MAX de las filas propias,
// source='derived_m1') en vez de una ventana fija -- si el job no corre por
// N dias (reinicio, caida), la siguiente corrida retoma justo donde quedo y
// cubre TODO lo que falto, no solo los ultimos 2/5 dias. Antes de la primera
// corrida (MAX es NULL) arranca desde epoch, pero en la practica esto nunca
// procesa "toda la historia": M1 solo llega a ~8.6 dias de profundidad
// (limite propio de TastyTrade/dxFeed), asi que el primer barrido esta
// acotado por ese mismo techo.
// date_trunc sobre un timestamptz usa el timezone de la SESION, no UTC --
// forzar "AT TIME ZONE 'UTC'" en ambas direcciones asegura que el corte de
// hora/dia caiga siempre en el mismo borde UTC que usa TastyTrade para sus
// D1 nativas (confirmado en vivo: siempre a las 00:00:00+00), sin importar
// el timezone configurado en la conexion.
// La segunda mitad (CTE "agg" -> INSERT INTO watermarks) deja registrado
// hasta donde quedo la agregacion de esta corrida, igual que Save() lo hace
// para cada vela M1 en vivo -- mismo mecanismo, disparado en el otro evento
// que pidio el usuario (medianoche) en vez de en cada guardado.
const aggregateH1SQL = `
	WITH watermark AS (
		SELECT COALESCE(MAX(ts), 'epoch'::timestamptz) AS wm
		FROM candles WHERE timeframe = 'H1' AND source = 'derived_m1'
	),
	agg AS (
		INSERT INTO candles (symbol_id, timeframe, ts, open, high, low, close, volume, trade_count, source)
		SELECT
			symbol_id, 'H1',
			date_trunc('hour', ts AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' AS bucket,
			(array_agg(open ORDER BY ts ASC))[1],
			max(high), min(low),
			(array_agg(close ORDER BY ts DESC))[1],
			sum(volume), sum(trade_count), 'derived_m1'
		FROM candles, watermark
		WHERE timeframe = 'M1'
			AND ts > watermark.wm
			AND date_trunc('hour', ts AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' + interval '1 hour' <= now()
		GROUP BY symbol_id, date_trunc('hour', ts AT TIME ZONE 'UTC')
		ON CONFLICT (symbol_id, timeframe, ts) DO UPDATE SET
			open = EXCLUDED.open, high = EXCLUDED.high, low = EXCLUDED.low, close = EXCLUDED.close,
			volume = EXCLUDED.volume, trade_count = EXCLUDED.trade_count
		RETURNING symbol_id, ts
	)
	INSERT INTO watermarks (symbol_id, timeframe, last_ts, updated_at)
	SELECT symbol_id, 'H1', max(ts), now() FROM agg GROUP BY symbol_id
	ON CONFLICT (symbol_id, timeframe) DO UPDATE SET
		last_ts = GREATEST(watermarks.last_ts, EXCLUDED.last_ts), updated_at = now()
`

const aggregateD1SQL = `
	WITH watermark AS (
		SELECT COALESCE(MAX(ts), 'epoch'::timestamptz) AS wm
		FROM candles WHERE timeframe = 'D1' AND source = 'derived_m1'
	),
	agg AS (
		INSERT INTO candles (symbol_id, timeframe, ts, open, high, low, close, volume, trade_count, source)
		SELECT
			symbol_id, 'D1',
			date_trunc('day', ts AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' AS bucket,
			(array_agg(open ORDER BY ts ASC))[1],
			max(high), min(low),
			(array_agg(close ORDER BY ts DESC))[1],
			sum(volume), sum(trade_count), 'derived_m1'
		FROM candles, watermark
		WHERE timeframe = 'M1'
			AND ts > watermark.wm
			AND date_trunc('day', ts AT TIME ZONE 'UTC') AT TIME ZONE 'UTC' + interval '1 day' <= now()
		GROUP BY symbol_id, date_trunc('day', ts AT TIME ZONE 'UTC')
		ON CONFLICT (symbol_id, timeframe, ts) DO UPDATE SET
			open = EXCLUDED.open, high = EXCLUDED.high, low = EXCLUDED.low, close = EXCLUDED.close,
			volume = EXCLUDED.volume, trade_count = EXCLUDED.trade_count
		RETURNING symbol_id, ts
	)
	INSERT INTO watermarks (symbol_id, timeframe, last_ts, updated_at)
	SELECT symbol_id, 'D1', max(ts), now() FROM agg GROUP BY symbol_id
	ON CONFLICT (symbol_id, timeframe) DO UPDATE SET
		last_ts = GREATEST(watermarks.last_ts, EXCLUDED.last_ts), updated_at = now()
`

func (r *CandleRepository) AggregateH1(ctx context.Context) error {
	err := execWithDeadlockRetry(ctx, func(ctx context.Context) error {
		_, err := r.pool.Exec(ctx, aggregateH1SQL)
		return err
	})
	if err != nil {
		return fmt.Errorf("aggregating H1 from M1: %w", err)
	}
	return nil
}

func (r *CandleRepository) AggregateD1(ctx context.Context) error {
	err := execWithDeadlockRetry(ctx, func(ctx context.Context) error {
		_, err := r.pool.Exec(ctx, aggregateD1SQL)
		return err
	})
	if err != nil {
		return fmt.Errorf("aggregating D1 from M1: %w", err)
	}
	return nil
}
