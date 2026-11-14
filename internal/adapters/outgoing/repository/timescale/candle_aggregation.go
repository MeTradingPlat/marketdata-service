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
// symbol_id resuelto con subconsulta escalar, no JOIN -- ver el comentario
// de seriesAggregatedBatchSQL mas abajo: con el JOIN, Postgres a veces
// descarta el indice (symbol_id, timeframe) y hace Seq Scan sobre todos los
// simbolos antes de filtrar.
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
		FROM %[1]s c
		WHERE c.symbol_id = (SELECT symbol_id FROM tracked_symbols WHERE symbol = $1)
			AND c.timeframe = $2 AND c.ts >= $4
			AND ($5::timestamptz IS NULL OR c.ts < $5)
		GROUP BY bucket
	) agg
	ORDER BY bucket DESC LIMIT $6
`

// rawCandlesForBucketingSQL: identica a la subconsulta interna de
// getCandlesSQL (candle_repository.go) pero sin el reordenado final a ASC
// -- aggregateIntoBuckets necesita orden DESC (ver su comentario), y solo
// hacen falta las columnas OHLCV, no vwap/source.
const rawCandlesForBucketingSQL = `
	SELECT c.ts, c.open, c.high, c.low, c.close, c.volume, c.trade_count
	FROM %[1]s c
	WHERE c.symbol_id = (SELECT symbol_id FROM tracked_symbols WHERE symbol = $1)
		AND c.timeframe = $2 AND c.ts >= $3
		AND ($4::timestamptz IS NULL OR c.ts < $4)
	ORDER BY c.ts DESC LIMIT $5
`

func (r *CandleRepository) queryRawCandles(ctx context.Context, symbol string, source domain.Timeframe, from time.Time, before *time.Time, limit int) ([]domain.Candle, error) {
	query := fmt.Sprintf(rawCandlesForBucketingSQL, candleTable(source))
	rows, err := r.pool.Query(ctx, query, symbol, string(source), from, before, limit)
	if err != nil {
		return nil, fmt.Errorf("querying raw candles for bucketing %s %s: %w", symbol, source, err)
	}
	defer rows.Close()

	raw := make([]domain.Candle, 0, limit)
	for rows.Next() {
		c := domain.Candle{Symbol: symbol}
		if err := rows.Scan(&c.Timestamp, &c.Open, &c.High, &c.Low, &c.Close, &c.Volume, &c.TradeCount); err != nil {
			return nil, fmt.Errorf("scanning raw candle for bucketing: %w", err)
		}
		raw = append(raw, c)
	}
	return raw, rows.Err()
}

func (r *CandleRepository) queryGroupedCandles(ctx context.Context, symbol string, timeframe, source domain.Timeframe, bucket string, from time.Time, before *time.Time, bars int) ([]domain.Candle, error) {
	query := fmt.Sprintf(getAggregatedCandlesSQL, candleTable(source))
	rows, err := r.pool.Query(ctx, query, symbol, string(source), bucket, from, before, bars)
	if err != nil {
		return nil, fmt.Errorf("querying aggregated candles for %s %s: %w", symbol, timeframe, err)
	}
	defer rows.Close()

	// candles arranca como slice vacio, no nil -- un nil slice serializa
	// como "null" en JSON en vez de "[]", rompiendo al frontend en
	// loadMoreHistory (ver el mismo comentario en GetCandles).
	candles := make([]domain.Candle, 0)
	for rows.Next() {
		c := domain.Candle{Symbol: symbol, Timeframe: timeframe, Source: "aggregated"}
		if err := rows.Scan(&c.Timestamp, &c.Open, &c.High, &c.Low, &c.Close, &c.Volume, &c.TradeCount); err != nil {
			return nil, fmt.Errorf("scanning aggregated candle row: %w", err)
		}
		candles = append(candles, c)
	}
	return candles, rows.Err()
}

func (r *CandleRepository) getAggregatedCandles(ctx context.Context, symbol string, timeframe, source domain.Timeframe, bucket string, approxPeriod time.Duration, bars int, before *time.Time) ([]domain.Candle, error) {
	anchor := before
	if anchor == nil {
		now := time.Now()
		anchor = &now
	}

	// sourcePeriod/ratio: cuando la fuente es M1/H1, se lee crudo y se agrupa
	// en Go (aggregateIntoBuckets) en vez de GROUP BY en SQL -- confirmado en
	// vivo el 2026-08-23, mismo motivo que GetSeriesAggregatedBatch (ver esa
	// funcion en este archivo).
	sourcePeriod, useRaw := sourcePeriodOf(source)
	ratio := 1
	if useRaw {
		ratio = int(approxPeriod / sourcePeriod)
		if ratio < 1 {
			ratio = 1
		}
	}

	candles := make([]domain.Candle, 0)
	window := time.Duration(bars*2+30) * approxPeriod
	// Mismo ensanchamiento de ventana que GetCandles (ver
	// candle_repository.go): una zona muerta grande deja la pagina en 0
	// velas y el frontend corta la paginacion creyendo que no hay mas.
	// prevCount corta apenas ensanchar deja de traer mas velas -- un simbolo
	// ralo (PCRX, CHRD: pocos minutos con trades reales) nunca junta `bars`
	// sin importar cuanto se agrande la ventana. Faltaba aca el mismo corte
	// que ya tiene GetCandles (confirmado en vivo el 2026-08-20 para ese
	// caso): sin el, las 4 vueltas de ensanche (hasta 256x la ventana base)
	// repetian la consulta completa sobre chunks comprimidos cada vez mas
	// viejos aunque ya no hubiera nada nuevo que encontrar -- confirmado en
	// vivo el 2026-08-21: 5-8s para cargar el grafico de un simbolo ralo,
	// contra <1s de uno liquido.
	prevCount := -1
	for attempt := 0; ; attempt++ {
		from := anchor.Add(-window)
		var err error
		if useRaw {
			rawLimit := bars*ratio*rawFetchMargin + 60
			var raw []domain.Candle
			raw, err = r.queryRawCandles(ctx, symbol, source, from, before, rawLimit)
			if err == nil {
				candles = aggregateIntoBuckets(raw, timeframe, approxPeriod, bars)
			}
		} else {
			candles, err = r.queryGroupedCandles(ctx, symbol, timeframe, source, bucket, from, before, bars)
		}
		if err != nil {
			return nil, err
		}
		if len(candles) >= bars || attempt >= maxWindowWidenAttempts || len(candles) == prevCount {
			break
		}
		prevCount = len(candles)
		window *= windowWidenFactor
	}

	// candles todavia esta en orden DESC aca (bucket DESC LIMIT $n de la
	// consulta) -- candles[0] es el mas reciente, el unico que puede seguir
	// asentandose.
	if before == nil && len(candles) > 0 {
		candles = r.dropUnsettledLastBucket(ctx, symbol, source, approxPeriod, candles)
	}

	for i, j := 0, len(candles)-1; i < j; i, j = i+1, j-1 {
		candles[i], candles[j] = candles[j], candles[i]
	}
	return candles, nil
}

// hasDataAtOrAfterSQL: existencia acotada, no MAX(ts)
const hasDataAtOrAfterSQL = `
	SELECT EXISTS (
		SELECT 1 FROM %[1]s c
		WHERE c.symbol_id = (SELECT symbol_id FROM tracked_symbols WHERE symbol = $1)
			AND c.timeframe = $2 AND c.ts >= $3
		LIMIT 1
	)
`

// dropUnsettledLastBucket confirma que ya llego al menos un M1 en o despues
// del cierre del bucket mas reciente antes de entregarlo como "cerrado".
func (r *CandleRepository) dropUnsettledLastBucket(ctx context.Context, symbol string, source domain.Timeframe, approxPeriod time.Duration, candles []domain.Candle) []domain.Candle {
	bucketEnd := candles[0].Timestamp.Add(approxPeriod)
	if bucketEnd.After(time.Now()) {
		// Todavia en formacion por reloj -- ni vale la pena esperar
		return candles[1:]
	}
	var settled bool
	query := fmt.Sprintf(hasDataAtOrAfterSQL, candleTable(source))
	err := r.pool.QueryRow(ctx, query, symbol, string(source), bucketEnd).Scan(&settled)
	if err != nil || settled {
		return candles
	}
	return candles[1:]
}
