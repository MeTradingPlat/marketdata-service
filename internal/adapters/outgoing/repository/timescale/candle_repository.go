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

// updateLastVolumeSQL solo se encola para velas D1 -- la comparacion contra
// last_volume_ts evita que el re-fetch defensivo del catch-up nocturno
// (10 dias hacia atras, ver incrementalMargin) pise el volumen mas
// reciente con uno mas viejo si llega despues en el mismo batch.
const updateLastVolumeSQL = `
	UPDATE tracked_symbols SET last_volume = $2, last_volume_ts = $3
	WHERE symbol = $1 AND (last_volume_ts IS NULL OR last_volume_ts < $3)
`

// Todo timeframe deja watermark aqui ahora -- antes solo M1 lo hacia (la
// vieja agregacion SQL H1/D1 ya no existe, asi que la fila ya no tiene con
// quien competir) y D1/H1 se apoyaban en MAX(ts) sobre candles directamente
// via GetWatermark. Eso resulto ser mucho mas caro de lo que parecia: sin
// un rango de fecha que acotar, Postgres no puede excluir chunks de la
// hypertable ni para PLANEAR la consulta (confirmado en vivo: hasta un
// EXPLAIN sin ejecutar tiraba "out of shared memory" con miles de chunks
// bloqueados de golpe). watermarks es una tabla chica sin particionar, una
// fila por simbolo+timeframe -- leerla es una busqueda por clave primaria,
// nunca toca la hypertable.
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
			if c.Timeframe == domain.D1 {
				batch.Queue(updateLastVolumeSQL, c.Symbol, c.Volume, c.Timestamp)
			}
		}
		results := r.pool.SendBatch(ctx, batch)
		defer results.Close()
		for _, c := range candles {
			if _, err := results.Exec(); err != nil {
				return fmt.Errorf("upserting candle batch: %w", err)
			}
			if _, err := results.Exec(); err != nil {
				return fmt.Errorf("upserting watermark batch: %w", err)
			}
			if c.Timeframe == domain.D1 {
				if _, err := results.Exec(); err != nil {
					return fmt.Errorf("updating last volume: %w", err)
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
		WHERE s.symbol = $1 AND c.timeframe = $2 AND c.ts >= $3
			AND ($4::timestamptz IS NULL OR c.ts < $4)
		ORDER BY c.ts DESC LIMIT $5
	) recent ORDER BY ts ASC
`

// GetCandles arranca leyendo el watermark (la tabla chica, ver comentario
// de Save()) para calcular un piso de fecha real antes de tocar la
// hypertable -- sin eso, "dame las ultimas N barras" no le daba a Postgres
// ningun rango que excluir y sufria el mismo problema que GetWatermark
// antes de arreglarse: hasta planear la consulta bloqueaba chunks de mas.
// El margen (2x bars + 30 periodos) cubre fines de semana/feriados/halts
// sin volver a un scan sin limite.
//
// before es nil para el chart inicial (las N barras mas recientes hasta el
// watermark) y no-nil para "cargar mas historial" al scrollear a la
// izquierda -- el frontend pide velas ANTES de la mas vieja que ya tiene
// (ver endDate en loadMoreHistory). Sin este parametro toda pagina
// devolvia siempre las mismas N barras mas recientes, asi que llegar al
// borde izquierdo nunca traia nada nuevo.
func (r *CandleRepository) GetCandles(ctx context.Context, symbol string, timeframe domain.Timeframe, bars int, before *time.Time) ([]domain.Candle, error) {
	if source, bucket, approxPeriod, ok := timeframe.Aggregation(); ok {
		return r.getAggregatedCandles(ctx, symbol, timeframe, source, bucket, approxPeriod, bars, before)
	}

	anchor := before
	if anchor == nil {
		newest, err := r.GetWatermark(ctx, symbol, timeframe)
		if err != nil {
			return nil, fmt.Errorf("checking watermark for %s %s: %w", symbol, timeframe, err)
		}
		if newest == nil {
			return nil, nil
		}
		anchor = newest
	}
	duration, err := timeframe.Duration()
	if err != nil {
		return nil, fmt.Errorf("getting duration for %s: %w", timeframe, err)
	}
	from := anchor.Add(-time.Duration(bars*2+30) * duration)

	rows, err := r.pool.Query(ctx, getCandlesSQL, symbol, string(timeframe), from, before, bars)
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

// watermarkSQL lee de la tabla watermarks (una fila por simbolo+timeframe,
// sin particionar) en vez de MAX(ts) sobre la hypertable -- ver el
// comentario de Save() sobre por que esa version se volvio carisima a esta
// escala.
const watermarkSQL = `
	SELECT w.last_ts
	FROM watermarks w JOIN tracked_symbols s ON s.symbol_id = w.symbol_id
	WHERE s.symbol = $1 AND w.timeframe = $2
`

// GetWatermark devuelve (nil, nil) si el simbolo+timeframe todavia no tiene
// ninguna vela guardada -- a diferencia de MAX(ts) sobre la hypertable (que
// siempre devuelve una fila, con NULL si no hay datos), una consulta comun
// contra watermarks no devuelve fila si nunca se escribio, asi que
// pgx.ErrNoRows es el caso normal de "simbolo nuevo", no un error real.
func (r *CandleRepository) GetWatermark(ctx context.Context, symbol string, timeframe domain.Timeframe) (newest *time.Time, err error) {
	err = r.pool.QueryRow(ctx, watermarkSQL, symbol, string(timeframe)).Scan(&newest)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying watermark for %s %s: %w", symbol, timeframe, err)
	}
	return newest, nil
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

// intradaySessionsSQL agrega las M1 de hoy en las 3 sesiones (pre/regular/
// post) en una sola pasada. Los limites de tiempo se calculan en Go
// (dayStart/dayEnd/marketOpen/marketClose, ya en America/New_York) y se
// mandan como parametros -- comparar c.ts contra un valor ya calculado usa
// el indice (symbol_id, timeframe, ts) de verdad; la version anterior
// filtraba con (c.ts AT TIME ZONE ...)::date = hoy, una expresion por fila
// que Postgres no puede indexar y tardaba 30s en producción. ARRAY_AGG(...)[1]
// es el modismo para "primer/ultimo valor segun ese orden" sin una
// subconsulta correlacionada por sesion.
const intradaySessionsSQL = `
	SELECT
		(ARRAY_AGG(c.open ORDER BY c.ts) FILTER (WHERE c.ts >= $4 AND c.ts < $5))[1],
		MAX(c.high) FILTER (WHERE c.ts >= $4 AND c.ts < $5),
		MIN(c.low) FILTER (WHERE c.ts >= $4 AND c.ts < $5),
		COALESCE(SUM(c.volume) FILTER (WHERE c.ts >= $4 AND c.ts < $5), 0),
		COALESCE(SUM(c.volume) FILTER (WHERE c.ts < $4), 0),
		(ARRAY_AGG(c.close ORDER BY c.ts DESC) FILTER (WHERE c.ts < $4))[1],
		COALESCE(SUM(c.volume) FILTER (WHERE c.ts >= $5), 0),
		(ARRAY_AGG(c.close ORDER BY c.ts DESC) FILTER (WHERE c.ts >= $5))[1]
	FROM candles c JOIN tracked_symbols s ON s.symbol_id = c.symbol_id
	WHERE s.symbol = $1 AND c.timeframe = 'M1' AND c.ts >= $2 AND c.ts < $3
`

func (r *CandleRepository) GetIntradaySessions(ctx context.Context, symbol string) (domain.IntradaySnapshot, error) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return domain.IntradaySnapshot{}, fmt.Errorf("loading America/New_York location: %w", err)
	}
	nowET := time.Now().In(loc)
	dayStart := time.Date(nowET.Year(), nowET.Month(), nowET.Day(), 0, 0, 0, 0, loc)
	dayEnd := dayStart.Add(24 * time.Hour)
	marketOpen := time.Date(nowET.Year(), nowET.Month(), nowET.Day(), 9, 30, 0, 0, loc)
	marketClose := time.Date(nowET.Year(), nowET.Month(), nowET.Day(), 16, 0, 0, 0, loc)

	snap := domain.IntradaySnapshot{Symbol: symbol}
	var open, high, low, preClose, postClose *float64
	err = r.pool.QueryRow(ctx, intradaySessionsSQL, symbol, dayStart, dayEnd, marketOpen, marketClose).Scan(
		&open, &high, &low, &snap.DayVolume,
		&snap.PreMarketVolume, &preClose,
		&snap.PostMarketVolume, &postClose,
	)
	if err != nil {
		return domain.IntradaySnapshot{}, fmt.Errorf("querying intraday sessions for %s: %w", symbol, err)
	}
	if open != nil {
		snap.Open = *open
	}
	if high != nil {
		snap.High = *high
	}
	if low != nil {
		snap.Low = *low
	}
	if preClose != nil {
		snap.PreMarketClose = *preClose
	}
	if postClose != nil {
		snap.PostMarketClose = *postClose
	}
	return snap, nil
}
