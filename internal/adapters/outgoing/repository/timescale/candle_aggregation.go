package timescale

import (
	"context"
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// getAggregatedCandlesSQL agrupa el timeframe base ($2) en buckets de ancho
// $3 (formato interval de Postgres, ej. "5 minutes") -- mismo patron
// acotado por fecha que getCandlesSQL (ver GetCandles), asi que la
// exclusion de chunks del hypertable sigue funcionando igual. time_bucket
// alinea a limites UTC por defecto (meses/anios incluidos, de forma
// calendario) sin necesidad de pasar un origin propio.
const getAggregatedCandlesSQL = `
	SELECT bucket, open, high, low, close, volume, trade_count FROM (
		SELECT
			time_bucket($3::interval, c.ts) AS bucket,
			(ARRAY_AGG(c.open ORDER BY c.ts ASC))[1] AS open,
			MAX(c.high) AS high,
			MIN(c.low) AS low,
			(ARRAY_AGG(c.close ORDER BY c.ts DESC))[1] AS close,
			SUM(c.volume) AS volume,
			SUM(c.trade_count) AS trade_count
		FROM candles c JOIN tracked_symbols s ON s.symbol_id = c.symbol_id
		WHERE s.symbol = $1 AND c.timeframe = $2 AND c.ts >= $4
			AND ($5::timestamptz IS NULL OR c.ts < $5)
		GROUP BY bucket
	) agg
	ORDER BY bucket DESC LIMIT $6
`

// continuousAggregateViews mapea el bucket ("5 minutes"/"15 minutes") a su
// continuous aggregate de TimescaleDB (ver schema.sql, candles_m5/m15) --
// solo M5/M15 lo tienen: son los timeframes que de verdad usan los
// escaneres y los que mas filas M1 agrupan por bucket. El resto de
// timeframes derivados (M2/M3/M10/M30/M45, H2-H12, D2-Y1) siguen por
// getAggregatedCandlesSQL -- agrupan pocas filas (base H1/D1) o se piden
// poco, no justifican otra vista materializada todavia.
var continuousAggregateViews = map[string]string{
	"5 minutes":  "candles_m5",
	"15 minutes": "candles_m15",
}

// continuousAggregateCandlesSQL lee del continuous aggregate ya calculado
// en vez de agrupar M1 cruda en cada consulta -- mismo contrato de
// resultado que getAggregatedCandlesSQL (bucket DESC + LIMIT, el llamador
// invierte a ASC), pero sin el GROUP BY: real-time aggregation (default de
// TimescaleDB) sigue devolviendo el tramo mas reciente aun no materializado
// con datos correctos, asi que no hay ventana con huecos mientras la
// politica de refresco se pone al dia.
const continuousAggregateCandlesSQL = `
	SELECT ca.bucket, ca.open, ca.high, ca.low, ca.close, ca.volume, ca.trade_count
	FROM %s ca JOIN tracked_symbols s ON s.symbol_id = ca.symbol_id
	WHERE s.symbol = $1 AND ca.bucket >= $2
		AND ($3::timestamptz IS NULL OR ca.bucket < $3)
	ORDER BY ca.bucket DESC LIMIT $4
`

// seriesAggregatedBatchSQL trae los ultimos $2 buckets por simbolo para
// TODO el lote en una sola consulta -- ROW_NUMBER particionado por simbolo
// encuentra los buckets mas recientes de cada uno sin importar cuantos
// huecos haya de por medio, asi que a diferencia de getAggregatedCandles no
// necesita ensanchar ventana ni watermark por simbolo. Confirmado en vivo
// el 2026-08-20 con EXPLAIN ANALYZE contra candles_m15 (1.44M filas): 2.1s
// para 8861 simbolos x 15 barras, contra los 14-15s que costaba el mismo
// pedido via GetCandlesBatch (una consulta por simbolo, 4 workers).
const seriesAggregatedBatchSQL = `
	SELECT symbol, bucket, open, high, low, close, volume, trade_count FROM (
		SELECT s.symbol, ca.bucket, ca.open, ca.high, ca.low, ca.close, ca.volume, ca.trade_count,
		       ROW_NUMBER() OVER (PARTITION BY ca.symbol_id ORDER BY ca.bucket DESC) AS rn
		FROM %s ca JOIN tracked_symbols s ON s.symbol_id = ca.symbol_id
		WHERE s.symbol = ANY($1)
	) t
	WHERE rn <= $2
	ORDER BY symbol, bucket
`

// GetSeriesAggregatedBatch es GetSeries (ver candle_repository.go) para un
// timeframe derivado con continuous aggregate -- solo cubre los buckets con
// vista propia (candles_m5/candles_m15, ver continuousAggregateViews). El
// caller decide el fallback per-simbolo para el resto de timeframes
// derivados.
func (r *CandleRepository) GetSeriesAggregatedBatch(ctx context.Context, symbols []string, timeframe domain.Timeframe, bucket string, bars int) (map[string][]domain.Candle, bool, error) {
	view, ok := continuousAggregateViews[bucket]
	if !ok {
		return nil, false, nil
	}
	if len(symbols) == 0 {
		return map[string][]domain.Candle{}, true, nil
	}

	rows, err := r.pool.Query(ctx, fmt.Sprintf(seriesAggregatedBatchSQL, view), symbols, bars)
	if err != nil {
		return nil, true, fmt.Errorf("querying aggregated series batch for %d symbols: %w", len(symbols), err)
	}
	defer rows.Close()

	result := make(map[string][]domain.Candle)
	for rows.Next() {
		var c domain.Candle
		if err := rows.Scan(&c.Symbol, &c.Timestamp, &c.Open, &c.High, &c.Low, &c.Close, &c.Volume, &c.TradeCount); err != nil {
			return nil, true, fmt.Errorf("scanning aggregated series batch row: %w", err)
		}
		c.Timeframe = timeframe
		c.Source = "aggregated"
		result[c.Symbol] = append(result[c.Symbol], c)
	}
	return result, true, rows.Err()
}

func (r *CandleRepository) getAggregatedCandles(ctx context.Context, symbol string, timeframe, source domain.Timeframe, bucket string, approxPeriod time.Duration, bars int, before *time.Time) ([]domain.Candle, error) {
	anchor := before
	if anchor == nil {
		newest, err := r.GetWatermark(ctx, symbol, source)
		if err != nil {
			return nil, fmt.Errorf("checking watermark for %s %s: %w", symbol, source, err)
		}
		if newest == nil {
			return []domain.Candle{}, nil
		}
		anchor = newest
	}
	query := getAggregatedCandlesSQL
	buildArgs := func(from time.Time) []any { return []any{symbol, string(source), bucket, from, before, bars} }
	if view, ok := continuousAggregateViews[bucket]; ok {
		query = fmt.Sprintf(continuousAggregateCandlesSQL, view)
		buildArgs = func(from time.Time) []any { return []any{symbol, from, before, bars} }
	}

	candles := make([]domain.Candle, 0)
	window := time.Duration(bars*2+30) * approxPeriod
	// Mismo ensanchamiento de ventana que GetCandles (ver
	// candle_repository.go): una zona muerta grande deja la pagina en 0
	// velas y el frontend corta la paginacion creyendo que no hay mas.
	for attempt := 0; ; attempt++ {
		from := anchor.Add(-window)
		rows, err := r.pool.Query(ctx, query, buildArgs(from)...)
		if err != nil {
			return nil, fmt.Errorf("querying aggregated candles for %s %s: %w", symbol, timeframe, err)
		}
		// candles arranca como slice vacio, no nil -- ver el mismo
		// comentario en GetCandles (candle_repository.go): un nil slice
		// serializa como "null" en JSON en vez de "[]", rompiendo al
		// frontend en loadMoreHistory.
		candles = candles[:0]
		for rows.Next() {
			c := domain.Candle{Symbol: symbol, Timeframe: timeframe, Source: "aggregated"}
			if err := rows.Scan(&c.Timestamp, &c.Open, &c.High, &c.Low, &c.Close, &c.Volume, &c.TradeCount); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scanning aggregated candle row: %w", err)
			}
			candles = append(candles, c)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		if len(candles) >= bars || attempt >= maxWindowWidenAttempts {
			break
		}
		window *= windowWidenFactor
	}

	for i, j := 0, len(candles)-1; i < j; i, j = i+1, j-1 {
		candles[i], candles[j] = candles[j], candles[i]
	}
	return candles, nil
}
