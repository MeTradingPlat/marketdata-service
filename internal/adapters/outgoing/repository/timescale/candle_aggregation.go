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
