package timescale

import (
	"context"
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

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
