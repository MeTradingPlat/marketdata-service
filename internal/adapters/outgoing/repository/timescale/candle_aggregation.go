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
		FROM candles c
		WHERE c.symbol_id = (SELECT symbol_id FROM tracked_symbols WHERE symbol = $1)
			AND c.timeframe = $2 AND c.ts >= $4
			AND ($5::timestamptz IS NULL OR c.ts < $5)
		GROUP BY bucket
	) agg
	ORDER BY bucket DESC LIMIT $6
`

// batchWindowMargin: sin ensanche de ventana (a diferencia de
// getAggregatedCandles) -- el batch acepta que un simbolo ralo devuelva
// menos velas de las pedidas en vez de pagar reintentos por simbolo (ver
// GetSeriesAggregatedBatch). x6 en vez de x2 (que usa el camino de un solo
// simbolo) cubre fines de semana/horas fuera de mercado sin ese reintento.
const batchWindowMargin = 6

// seriesAggregatedBatchSQL agrupa el timeframe base ($3) crudo en buckets
// de ancho $4 y trae los ultimos $2 por simbolo para TODO el lote en una
// sola consulta -- mismo patron time_bucket que getAggregatedCandlesSQL,
// pero con JOIN LATERAL en vez de la subconsulta escalar (un solo simbolo
// por consulta ahi) porque aca hay que resolver el lote entero de una vez.
// Solo se usa cuando el timeframe base es D1 (ver sourcePeriodOf) -- M1/H1
// usan seriesRawBatchSQL + aggregateIntoBuckets (mucho mas rapido, ver esa
// consulta mas abajo).
//
// JOIN LATERAL, no JOIN + ROW_NUMBER -- confirmado en vivo el 2026-08-21
// con EXPLAIN ANALYZE (entonces contra el continuous aggregate, mismo
// principio aca contra la tabla cruda): la version con JOIN hacia que
// Postgres descartara el indice (symbol_id, timeframe, ts) y escaneara TODA
// la tabla antes de filtrar por el lote pedido. Con LATERAL, Postgres hace
// un Index Scan real por simbolo, acotado ademas por c.ts >= $5
// (batchWindowMargin) para no agrupar mas M1 de la necesaria.
const seriesAggregatedBatchSQL = `
	SELECT s.symbol, agg.bucket, agg.open, agg.high, agg.low, agg.close, agg.volume, agg.trade_count
	FROM tracked_symbols s
	JOIN LATERAL (
		SELECT bucket, open, high, low, close, volume, trade_count FROM (
			SELECT
				time_bucket($4::interval, c.ts) AS bucket,
				(ARRAY_AGG(c.open ORDER BY c.ts ASC))[1] AS open,
				MAX(c.high) AS high,
				MIN(c.low) AS low,
				(ARRAY_AGG(c.close ORDER BY c.ts DESC))[1] AS close,
				SUM(c.volume) AS volume,
				SUM(c.trade_count) AS trade_count
			FROM candles c
			WHERE c.symbol_id = s.symbol_id AND c.timeframe = $3 AND c.ts >= $5
			GROUP BY bucket
			ORDER BY bucket DESC
			LIMIT $2
		) b
	) agg ON true
	WHERE s.symbol = ANY($1)
	ORDER BY s.symbol, agg.bucket
`

// seriesRawBatchSQL trae las ultimas $2 velas CRUDAS por simbolo (sin
// GROUP BY) para que la agregacion en buckets la haga aggregateIntoBuckets
// en Go -- confirmado en vivo el 2026-08-23 con EXPLAIN ANALYZE sobre 700
// simbolos reales: la version con GROUP BY+ARRAY_AGG+Sort de
// seriesAggregatedBatchSQL tardaba 1064ms contra 75ms de esta misma lectura
// sin agrupar (14x), porque ARRAY_AGG+GroupAggregate+Sort le cuesta a
// Postgres mucho mas que un Index Scan simple -- el trabajo de agrupar 5-60x
// menos filas en un loop de Go es órdenes de magnitud mas barato. Devuelve
// en orden DESC (mismo orden que espera aggregateIntoBuckets).
const seriesRawBatchSQL = `
	SELECT s.symbol, raw.ts, raw.open, raw.high, raw.low, raw.close, raw.volume, raw.trade_count
	FROM tracked_symbols s
	JOIN LATERAL (
		SELECT c.ts, c.open, c.high, c.low, c.close, c.volume, c.trade_count
		FROM candles c
		WHERE c.symbol_id = s.symbol_id AND c.timeframe = $3 AND c.ts >= $4
		ORDER BY c.ts DESC
		LIMIT $2
	) raw ON true
	WHERE s.symbol = ANY($1)
	ORDER BY s.symbol, raw.ts DESC
`

// rawFetchMargin: cuantas veces bars*ratio de filas crudas se piden de mas,
// para no quedar cortos por huecos (fin de semana, mercado cerrado) dentro
// de la ventana de fecha -- mismo espiritu que windowWidenFactor, pero como
// tope de LIMIT en vez de ensanche de ventana (el batch no reintenta por
// simbolo, ver el comentario de batchWindowMargin).
const rawFetchMargin = 4

// GetSeriesAggregatedBatch es GetSeries (ver candle_repository.go) para un
// timeframe derivado -- agrega el timeframe base (source/bucket/approxPeriod,
// ver domain.Timeframe.Aggregation) on-the-fly para TODO el lote en una sola
// consulta, sin depender de un continuous aggregate materializado (retirado:
// solo cubria M5/M15 y su politica de refresco no backfillea historia
// vieja, confirmado en vivo el 2026-08-23 como causa de un hueco real de
// datos en M5). Cuando el timeframe base es M1/H1 agrega crudo en Go
// (mucho mas rapido, ver seriesRawBatchSQL); D1 sigue agregandose en SQL
// (ver sourcePeriodOf).
func (r *CandleRepository) GetSeriesAggregatedBatch(ctx context.Context, symbols []string, timeframe, source domain.Timeframe, bucket string, approxPeriod time.Duration, bars int) (map[string][]domain.Candle, error) {
	if len(symbols) == 0 {
		return map[string][]domain.Candle{}, nil
	}
	since := time.Now().Add(-time.Duration(bars*batchWindowMargin+60) * approxPeriod)

	if sourcePeriod, ok := sourcePeriodOf(source); ok {
		return r.getSeriesAggregatedBatchRaw(ctx, symbols, timeframe, source, approxPeriod, sourcePeriod, bars, since)
	}

	rows, err := r.pool.Query(ctx, seriesAggregatedBatchSQL, symbols, bars, string(source), bucket, since)
	if err != nil {
		return nil, fmt.Errorf("querying aggregated series batch for %d symbols: %w", len(symbols), err)
	}
	defer rows.Close()

	result := make(map[string][]domain.Candle)
	for rows.Next() {
		var c domain.Candle
		if err := rows.Scan(&c.Symbol, &c.Timestamp, &c.Open, &c.High, &c.Low, &c.Close, &c.Volume, &c.TradeCount); err != nil {
			return nil, fmt.Errorf("scanning aggregated series batch row: %w", err)
		}
		c.Timeframe = timeframe
		c.Source = "aggregated"
		result[c.Symbol] = append(result[c.Symbol], c)
	}
	return result, rows.Err()
}

func (r *CandleRepository) getSeriesAggregatedBatchRaw(ctx context.Context, symbols []string, timeframe, source domain.Timeframe, approxPeriod, sourcePeriod time.Duration, bars int, since time.Time) (map[string][]domain.Candle, error) {
	ratio := int(approxPeriod / sourcePeriod)
	if ratio < 1 {
		ratio = 1
	}
	rawLimit := bars*ratio*rawFetchMargin + 60

	rows, err := r.pool.Query(ctx, seriesRawBatchSQL, symbols, rawLimit, string(source), since)
	if err != nil {
		return nil, fmt.Errorf("querying raw series batch for %d symbols: %w", len(symbols), err)
	}
	defer rows.Close()

	rawBySymbol := make(map[string][]domain.Candle)
	for rows.Next() {
		var c domain.Candle
		if err := rows.Scan(&c.Symbol, &c.Timestamp, &c.Open, &c.High, &c.Low, &c.Close, &c.Volume, &c.TradeCount); err != nil {
			return nil, fmt.Errorf("scanning raw series batch row: %w", err)
		}
		rawBySymbol[c.Symbol] = append(rawBySymbol[c.Symbol], c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make(map[string][]domain.Candle, len(rawBySymbol))
	for symbol, raw := range rawBySymbol {
		buckets := aggregateIntoBuckets(raw, timeframe, approxPeriod, bars)
		for i, j := 0, len(buckets)-1; i < j; i, j = i+1, j-1 {
			buckets[i], buckets[j] = buckets[j], buckets[i]
		}
		result[symbol] = buckets
	}
	return result, nil
}

// rawCandlesForBucketingSQL: identica a la subconsulta interna de
// getCandlesSQL (candle_repository.go) pero sin el reordenado final a ASC
// -- aggregateIntoBuckets necesita orden DESC (ver su comentario), y solo
// hacen falta las columnas OHLCV, no vwap/source.
const rawCandlesForBucketingSQL = `
	SELECT c.ts, c.open, c.high, c.low, c.close, c.volume, c.trade_count
	FROM candles c
	WHERE c.symbol_id = (SELECT symbol_id FROM tracked_symbols WHERE symbol = $1)
		AND c.timeframe = $2 AND c.ts >= $3
		AND ($4::timestamptz IS NULL OR c.ts < $4)
	ORDER BY c.ts DESC LIMIT $5
`

func (r *CandleRepository) queryRawCandles(ctx context.Context, symbol string, source domain.Timeframe, from time.Time, before *time.Time, limit int) ([]domain.Candle, error) {
	rows, err := r.pool.Query(ctx, rawCandlesForBucketingSQL, symbol, string(source), from, before, limit)
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
	rows, err := r.pool.Query(ctx, getAggregatedCandlesSQL, symbol, string(source), bucket, from, before, bars)
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
		newest, err := r.GetWatermark(ctx, symbol, source)
		if err != nil {
			return nil, fmt.Errorf("checking watermark for %s %s: %w", symbol, source, err)
		}
		if newest == nil {
			return []domain.Candle{}, nil
		}
		anchor = newest
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

const (
	settleCheckRetries  = 2
	settleCheckInterval = time.Second
)

// hasDataAtOrAfterSQL: existencia acotada, no MAX(ts) -- Save() en vivo
// (StreamLive) guarda cada vela M1 con withWatermark=false a proposito (una
// vela en formacion puede llegar con datos parciales/erroneos que no deben
// quedar como punto de retomada del backfill), asi que la tabla watermarks
// NUNCA avanza durante el dia de trading, solo en el catchup/backfill -- un
// intento anterior de este mismo fix uso GetWatermark() y quedaba siempre
// desactualizado en vivo, descartando la ultima vela SIEMPRE, sin importar
// si de verdad estaba asentada. `ts >= $3 LIMIT 1` cae en el chunk mas
// reciente sin comprimir (bucketEnd siempre es reciente) en vez de escanear
// toda la hypertable como haria MAX(ts) -- mismo motivo por el que
// watermarks existe en primer lugar (ver comentario de Save()).
const hasDataAtOrAfterSQL = `
	SELECT EXISTS (
		SELECT 1 FROM candles c
		WHERE c.symbol_id = (SELECT symbol_id FROM tracked_symbols WHERE symbol = $1)
			AND c.timeframe = $2 AND c.ts >= $3
		LIMIT 1
	)
`

// dropUnsettledLastBucket confirma que ya llego al menos un M1 en o despues
// del cierre del bucket mas reciente antes de entregarlo como "cerrado" --
// mismo patron de "watermark" de sistemas de streaming (Flink/Spark/Kafka
// Streams): un resultado de ventana solo se entrega una vez que se
// confirmo que no puede llegar mas data para ella. Sin esto, un bucket
// "cerrado por reloj" (timestamp+periodo <= ahora) podia devolverse con
// datos incompletos si el ultimo M1 que lo compone todavia no habia
// llegado a la BD -- confirmado en vivo con NMAX: el precio de la señal
// quedo en el high de un momento a medio completar, no en el close final.
// Reintenta un par de segundos (la vela SIGUIENTE ya deberia estar
// entrando) antes de descartarlo -- mucho mas corto y preciso que un
// margen de tiempo fijo adivinado.
func (r *CandleRepository) dropUnsettledLastBucket(ctx context.Context, symbol string, source domain.Timeframe, approxPeriod time.Duration, candles []domain.Candle) []domain.Candle {
	bucketEnd := candles[0].Timestamp.Add(approxPeriod)
	if bucketEnd.After(time.Now()) {
		// Todavia en formacion por reloj -- ni vale la pena esperar, no es
		// una vela "cerrada" en ningun sentido todavia. La sirve el
		// mecanismo de vela en vivo (seed/forwardLive del WS), no el
		// historial -- sin este corte temprano, CUALQUIER carga de grafico
		// en horario de mercado (el bucket mas reciente casi siempre sigue
		// en formacion) pagaria los reintentos de mas abajo por nada.
		return candles[1:]
	}
	for attempt := 0; attempt <= settleCheckRetries; attempt++ {
		var settled bool
		err := r.pool.QueryRow(ctx, hasDataAtOrAfterSQL, symbol, string(source), bucketEnd).Scan(&settled)
		if err != nil || settled {
			// err != nil: no bloquear la respuesta por un chequeo que fallo,
			// se confia en los datos ya traidos.
			return candles
		}
		if attempt < settleCheckRetries {
			time.Sleep(settleCheckInterval)
		}
	}
	return candles[1:]
}
