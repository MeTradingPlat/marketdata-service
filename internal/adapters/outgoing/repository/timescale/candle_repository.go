package timescale

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
		for range candles {
			if _, err := results.Exec(); err != nil {
				return fmt.Errorf("upserting candle batch: %w", err)
			}
			if withWatermark {
				if _, err := results.Exec(); err != nil {
					return fmt.Errorf("upserting watermark batch: %w", err)
				}
			}
		}
		for range latestDailies {
			if _, err := results.Exec(); err != nil {
				return fmt.Errorf("updating last volume: %w", err)
			}
		}
		return nil
	})
}

// symbol_id resuelto con subconsulta escalar, no JOIN -- ver el comentario
// de seriesAggregatedBatchSQL (candle_aggregation.go): con el JOIN,
// Postgres a veces descarta el indice (symbol_id, timeframe) y hace Seq
// Scan sobre todos los simbolos antes de filtrar.
const getCandlesSQL = `
	SELECT ts, open, high, low, close, volume, trade_count, vwap, source FROM (
		SELECT c.ts, c.open, c.high, c.low, c.close, c.volume, c.trade_count, c.vwap, c.source
		FROM %[1]s c
		WHERE c.symbol_id = (SELECT symbol_id FROM tracked_symbols WHERE symbol = $1)
			AND c.timeframe = $2 AND c.ts >= $3
			AND ($4::timestamptz IS NULL OR c.ts < $4)
		ORDER BY c.ts DESC LIMIT $5
	) recent ORDER BY ts ASC
`

// maxWindowWidenAttempts/windowWidenFactor acotan el ensanchamiento de la ventana en GetCandles.
const (
	maxWindowWidenAttempts = 4
	windowWidenFactor      = 4
)

// GetCandles lee el watermark para acotar el rango antes de tocar la hypertable; el margen base cubre feriados/halts y las zonas muertas se absorben ensanchando la ventana.
//
// before es nil para el chart inicial y no-nil para cargar historial anterior al scrollear a la izquierda.
// GetSeries lee hasta bars velas por simbolo para un lote en una sola query, devueltas ordenadas por ts ascendente.
func (r *CandleRepository) GetSeries(ctx context.Context, symbols []string, timeframe domain.Timeframe, bars int) (map[string][]domain.Candle, error) {
	return getSeriesFrom(ctx, r.pool, symbols, timeframe, bars)
}

// GetSeriesPriority es GetSeries por el pool chico de fundamentals/realtime (ver storage.snapshotPoolConns).
func (r *CandleRepository) GetSeriesPriority(ctx context.Context, symbols []string, timeframe domain.Timeframe, bars int) (map[string][]domain.Candle, error) {
	return getSeriesFrom(ctx, r.snapshotPool, symbols, timeframe, bars)
}

// seriesLookbackWindow calcula cuanto retroceder para encontrar bars velas; la cota de ts deja que TimescaleDB excluya chunks viejos enteros sin descomprimirlos.
func seriesLookbackWindow(bars int, duration time.Duration) time.Duration {
	const minLookback = 10 * 24 * time.Hour
	var window time.Duration
	if duration >= 24*time.Hour {
		window = time.Duration(float64(bars)*1.6+60) * duration
	} else {
		window = time.Duration(bars*3+60) * duration
	}
	return max(window, minLookback)
}

const getSeriesSinceSQL = `
		SELECT s.symbol, ts, open, high, low, close, volume, source
		FROM (
			SELECT c.symbol_id, c.ts, c.open, c.high, c.low, c.close, c.volume, c.source,
			       row_number() OVER (PARTITION BY c.symbol_id ORDER BY c.ts DESC) AS rn
			FROM %[1]s c
			WHERE c.timeframe = $1 AND c.ts >= $4
			  AND c.symbol_id = ANY(ARRAY(SELECT symbol_id FROM tracked_symbols WHERE symbol = ANY($2)))
		) t
		JOIN tracked_symbols s ON s.symbol_id = t.symbol_id
		WHERE rn <= $3
		ORDER BY s.symbol, ts`

func getSeriesSince(ctx context.Context, pool *pgxpool.Pool, symbols []string, timeframe domain.Timeframe, bars int, from time.Time) (map[string][]domain.Candle, error) {
	query := fmt.Sprintf(getSeriesSinceSQL, candleTable(timeframe))
	rows, err := pool.Query(ctx, query,
		string(timeframe), symbols, bars, from)
	if err != nil {
		return nil, fmt.Errorf("loading %s series for %d symbols: %w", timeframe, len(symbols), err)
	}
	defer rows.Close()

	result := make(map[string][]domain.Candle)
	for rows.Next() {
		var c domain.Candle
		if err := rows.Scan(&c.Symbol, &c.Timestamp, &c.Open, &c.High, &c.Low, &c.Close, &c.Volume, &c.Source); err != nil {
			return nil, fmt.Errorf("scanning series candle: %w", err)
		}
		c.Timeframe = timeframe
		result[c.Symbol] = append(result[c.Symbol], c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating series rows: %w", err)
	}
	return result, nil
}

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
			return []domain.Candle{}, nil
		}
		anchor = newest
	}
	duration, err := timeframe.Duration()
	if err != nil {
		return nil, fmt.Errorf("getting duration for %s: %w", timeframe, err)
	}

	candles := make([]domain.Candle, 0)
	window := time.Duration(bars*2+30) * duration
	// Si la pagina viene corta se ensancha la ventana (x4) y se reintenta, para no confundir una zona muerta con "no hay mas historia".
	prevCount := -1
	query := fmt.Sprintf(getCandlesSQL, candleTable(timeframe))
	for attempt := 0; ; attempt++ {
		from := anchor.Add(-window)
		rows, err := r.pool.Query(ctx, query, symbol, string(timeframe), from, before, bars)
		if err != nil {
			return nil, fmt.Errorf("querying candles for %s %s: %w", symbol, timeframe, err)
		}
		// candles arranca como slice vacio, no nil, para serializar "[]" y no "null" en la respuesta JSON.
		candles = candles[:0]
		for rows.Next() {
			c := domain.Candle{Symbol: symbol, Timeframe: timeframe}
			if err := rows.Scan(&c.Timestamp, &c.Open, &c.High, &c.Low, &c.Close, &c.Volume, &c.TradeCount, &c.VWAP, &c.Source); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scanning candle row: %w", err)
			}
			candles = append(candles, c)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		// Un simbolo con historia real corta nunca junta bars: se corta si ya hay velas y no crecen, o al agotar los intentos permitidos.
		if len(candles) >= bars || attempt >= maxWindowWidenAttempts || (len(candles) > 0 && len(candles) == prevCount) {
			return candles, nil
		}
		prevCount = len(candles)
		window *= windowWidenFactor
	}
}

// prevSessionCloseDays son cuantos dias de calendario hacia atras se buscan
// ventanas de cierre regular -- 10 cubre cualquier racha sin mercado
// (fines de semana + feriados encadenados, ej. Thanksgiving + fin de
// semana) con ventanas de 4 minutos por dia: 10 busquedas por indice
// (symbol_id, timeframe, ts), no un scan de toda la historia.
const prevSessionCloseDays = 10

// GetPreviousSessionClose: ver el puerto (ports/out/repositories.go). Las
// ventanas se calculan en America/New_York (la subasta de cierre cae a las
// 16:00 ET tambien en horario de verano) y se mandan como parametros --
// el planner de Postgres puede usar el indice por cada rama del OR, igual
// que el resto de consultas de este repositorio.
func (r *CandleRepository) GetPreviousSessionClose(ctx context.Context, symbol string, before time.Time) (*float64, error) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return nil, fmt.Errorf("loading America/New_York location: %w", err)
	}
	dayStart := time.Date(before.Year(), before.Month(), before.Day(), 0, 0, 0, 0, loc)

	args := []any{symbol, before}
	windows := make([]string, 0, prevSessionCloseDays)
	for d := 1; d <= prevSessionCloseDays; d++ {
		day := dayStart.AddDate(0, 0, -d)
		from := day.Add(15*time.Hour + 58*time.Minute)
		// to = 16:01 ET: la ts=16:00 es la subasta de cierre, la 16:01 ya es after-hours.
		to := day.Add(16*time.Hour + time.Minute)
		windows = append(windows, fmt.Sprintf("($%d::timestamptz <= c.ts AND c.ts < $%d::timestamptz)", len(args)+1, len(args)+2))
		args = append(args, from, to)
	}

	query := fmt.Sprintf(`
		SELECT c.close
		FROM candles c JOIN tracked_symbols s ON s.symbol_id = c.symbol_id
		WHERE s.symbol = $1 AND c.timeframe = 'M1' AND c.ts < $2
			AND (%s)
		ORDER BY c.ts DESC LIMIT 1`, strings.Join(windows, " OR "))

	var close float64
	err = r.pool.QueryRow(ctx, query, args...).Scan(&close)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying previous session close for %s: %w", symbol, err)
	}
	return &close, nil
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

// GetWatermarksBatch lee los watermarks de un lote en una sola query.
func (r *CandleRepository) GetWatermarksBatch(ctx context.Context, symbols []string, timeframe domain.Timeframe) (map[string]time.Time, error) {
	if len(symbols) == 0 {
		return map[string]time.Time{}, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT s.symbol, w.last_ts
		FROM watermarks w
		JOIN tracked_symbols s ON s.symbol_id = w.symbol_id
		WHERE w.timeframe = $1 AND s.symbol = ANY($2)`, string(timeframe), symbols)
	if err != nil {
		return nil, fmt.Errorf("querying watermarks batch: %w", err)
	}
	defer rows.Close()
	result := make(map[string]time.Time, len(symbols))
	for rows.Next() {
		var symbol string
		var last time.Time
		if err := rows.Scan(&symbol, &last); err != nil {
			return nil, fmt.Errorf("scanning watermark batch: %w", err)
		}
		result[symbol] = last
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating watermark batch: %w", err)
	}
	return result, nil
}

const symbolsWithDataSQL = `
	SELECT s.symbol
	FROM watermarks w JOIN tracked_symbols s ON s.symbol_id = w.symbol_id
	WHERE w.timeframe = $1
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

// intradaySessionsSQL agrega las M1 de hoy en las 3 sesiones en una sola pasada; los limites se calculan en Go para usar el indice (symbol_id, timeframe, ts).
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

// GetIntradaySessionsBatch es intradaySessionsSQL con GROUP BY s.symbol.
// universeBatchChunkSize acota cuantos simbolos entran en un solo ANY($1) para limitar el pico de memoria por consulta.
const universeBatchChunkSize = 1500

func (r *CandleRepository) GetIntradaySessionsBatch(ctx context.Context, symbols []string) (map[string]domain.IntradaySnapshot, error) {
	result := make(map[string]domain.IntradaySnapshot, len(symbols))
	if len(symbols) == 0 {
		return result, nil
	}
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return nil, fmt.Errorf("loading America/New_York location: %w", err)
	}
	nowET := time.Now().In(loc)
	dayStart := time.Date(nowET.Year(), nowET.Month(), nowET.Day(), 0, 0, 0, 0, loc)
	dayEnd := dayStart.Add(24 * time.Hour)
	marketOpen := time.Date(nowET.Year(), nowET.Month(), nowET.Day(), 9, 30, 0, 0, loc)
	marketClose := time.Date(nowET.Year(), nowET.Month(), nowET.Day(), 16, 0, 0, 0, loc)

	for _, batch := range chunkSymbols(symbols, universeBatchChunkSize) {
		if err := r.queryIntradaySessionsBatch(ctx, batch, dayStart, dayEnd, marketOpen, marketClose, result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (r *CandleRepository) queryIntradaySessionsBatch(ctx context.Context, symbols []string, dayStart, dayEnd, marketOpen, marketClose time.Time, result map[string]domain.IntradaySnapshot) error {
	query := `
		SELECT
			s.symbol,
			(ARRAY_AGG(c.open ORDER BY c.ts) FILTER (WHERE c.ts >= $4 AND c.ts < $5))[1],
			MAX(c.high) FILTER (WHERE c.ts >= $4 AND c.ts < $5),
			MIN(c.low) FILTER (WHERE c.ts >= $4 AND c.ts < $5),
			COALESCE(SUM(c.volume) FILTER (WHERE c.ts >= $4 AND c.ts < $5), 0),
			COALESCE(SUM(c.volume) FILTER (WHERE c.ts < $4), 0),
			(ARRAY_AGG(c.close ORDER BY c.ts DESC) FILTER (WHERE c.ts < $4))[1],
			COALESCE(SUM(c.volume) FILTER (WHERE c.ts >= $5), 0),
			(ARRAY_AGG(c.close ORDER BY c.ts DESC) FILTER (WHERE c.ts >= $5))[1]
		FROM candles c JOIN tracked_symbols s ON s.symbol_id = c.symbol_id
		WHERE s.symbol = ANY($1) AND c.timeframe = 'M1' AND c.ts >= $2 AND c.ts < $3
		GROUP BY s.symbol
	`
	rows, err := r.snapshotPool.Query(ctx, query, symbols, dayStart, dayEnd, marketOpen, marketClose)
	if err != nil {
		return fmt.Errorf("querying intraday sessions batch for %d symbols: %w", len(symbols), err)
	}
	defer rows.Close()

	for rows.Next() {
		var snap domain.IntradaySnapshot
		var open, high, low, preClose, postClose *float64
		if err := rows.Scan(&snap.Symbol, &open, &high, &low, &snap.DayVolume,
			&snap.PreMarketVolume, &preClose, &snap.PostMarketVolume, &postClose); err != nil {
			return fmt.Errorf("scanning intraday sessions batch row: %w", err)
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
		result[snap.Symbol] = snap
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating intraday sessions batch rows: %w", err)
	}
	return nil
}

// previousSessionCloseWindowSQL busca la subasta de UN dia para el lote entero; una ventana angosta por llamada solo toca 1-2 chunks.
const previousSessionCloseWindowSQL = `
	SELECT DISTINCT ON (s.symbol) s.symbol, c.close
	FROM candles c JOIN tracked_symbols s ON s.symbol_id = c.symbol_id
	WHERE s.symbol = ANY($1) AND c.timeframe = 'M1' AND c.ts >= $2 AND c.ts < $3
	ORDER BY s.symbol, c.ts DESC
`

// GetPreviousSessionCloseBatch es GetPreviousSessionClose para un lote --
// itera las mismas ventanas [15:58,16:01) ET dia por dia (ver
// prevSessionCloseDays), pero con una query angosta por dia en vez de un
// solo OR gigante (ver previousSessionCloseWindowSQL) y se corta apenas
// todos los simbolos pedidos ya resolvieron -- en la practica casi todos
// caen en la primera vuelta (el dia habil mas reciente), asi que el caso
// comun paga una sola query angosta, no diez.
func (r *CandleRepository) GetPreviousSessionCloseBatch(ctx context.Context, symbols []string, before time.Time) (map[string]float64, error) {
	result := make(map[string]float64, len(symbols))
	if len(symbols) == 0 {
		return result, nil
	}
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return nil, fmt.Errorf("loading America/New_York location: %w", err)
	}
	dayStart := time.Date(before.Year(), before.Month(), before.Day(), 0, 0, 0, 0, loc)

	pending := make([]string, len(symbols))
	copy(pending, symbols)
	for d := 1; d <= prevSessionCloseDays && len(pending) > 0; d++ {
		day := dayStart.AddDate(0, 0, -d)
		from := day.Add(15*time.Hour + 58*time.Minute)
		to := day.Add(16*time.Hour + time.Minute)

		resolved := make(map[string]struct{}, len(pending))
		for _, batch := range chunkSymbols(pending, universeBatchChunkSize) {
			rows, err := r.snapshotPool.Query(ctx, previousSessionCloseWindowSQL, batch, from, to)
			if err != nil {
				return nil, fmt.Errorf("querying previous session close window (day -%d) for %d symbols: %w", d, len(batch), err)
			}
			for rows.Next() {
				var symbol string
				var close float64
				if err := rows.Scan(&symbol, &close); err != nil {
					rows.Close()
					return nil, fmt.Errorf("scanning previous session close window row: %w", err)
				}
				result[symbol] = close
				resolved[symbol] = struct{}{}
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return nil, fmt.Errorf("iterating previous session close window rows: %w", err)
			}
			rows.Close()
		}

		if len(resolved) == 0 {
			continue
		}
		next := pending[:0]
		for _, sym := range pending {
			if _, ok := resolved[sym]; !ok {
				next = append(next, sym)
			}
		}
		pending = next
	}
	return result, nil
}
