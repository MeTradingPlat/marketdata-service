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
// de continuousAggregateCandlesSQL mas abajo: con el JOIN, Postgres a veces
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
//
// symbol_id resuelto con una SUBCONSULTA escalar, no un JOIN -- confirmado
// en vivo el 2026-08-21 con EXPLAIN ANALYZE: la version con JOIN hacia que
// el planner de Postgres descartara el indice (symbol_id, bucket) que SI
// existe sobre la tabla materializada y escaneara TODA la tabla (Seq Scan,
// 2.85 millones de filas de TODOS los simbolos, 2.3s) para despues recien
// filtrar por symbol_id via el join. Con la subconsulta, Postgres resuelve
// symbol_id primero y usa Index Scan directo: 44ms, 52x mas rapido, mismo
// resultado. Esto explicaba la mayor parte de la lentitud de M5/M15 que se
// veia en el chart y en las señales, no la logica de Go alrededor.
const continuousAggregateCandlesSQL = `
	SELECT ca.bucket, ca.open, ca.high, ca.low, ca.close, ca.volume, ca.trade_count
	FROM %s ca
	WHERE ca.symbol_id = (SELECT symbol_id FROM tracked_symbols WHERE symbol = $1)
		AND ca.bucket >= $2
		AND ($3::timestamptz IS NULL OR ca.bucket < $3)
	ORDER BY ca.bucket DESC LIMIT $4
`

// seriesAggregatedBatchSQL trae los ultimos $2 buckets por simbolo para
// TODO el lote en una sola consulta -- a diferencia de getAggregatedCandles
// no necesita ensanchar ventana ni watermark por simbolo, cualquier hueco
// de un simbolo no afecta a los demas.
//
// JOIN LATERAL, no JOIN + ROW_NUMBER -- confirmado en vivo el 2026-08-21
// con EXPLAIN ANALYZE: la version con JOIN hacia que Postgres descartara el
// indice (symbol_id, bucket) y escaneara TODA la tabla materializada (Seq
// Scan sobre 2.85 millones de filas de TODOS los simbolos) para despues
// filtrar por el lote pedido -- el "2.1s para 8861 simbolos" del commit
// original de esta consulta ya pagaba ese costo, solo que quedaba oculto
// porque el universo completo de todas formas necesitaba leer casi toda la
// tabla. Con LATERAL, Postgres hace un Index Scan real por simbolo (uno a
// la vez, pero cada uno acotado por el indice) -- medido con el tamano de
// lote y bars real de signal-processing (700 simbolos, 15-150 barras):
// 300-400ms, 10-13x mas rapido que el JOIN, y a diferencia del camino de UN
// simbolo (continuousAggregateCandlesSQL) esto SI escala bien de lote chico
// a lote de 700 porque el costo por simbolo es bajo con bars_needed
// tipico (docenas, no cientos).
const seriesAggregatedBatchSQL = `
	SELECT s.symbol, ca.bucket, ca.open, ca.high, ca.low, ca.close, ca.volume, ca.trade_count
	FROM tracked_symbols s
	JOIN LATERAL (
		SELECT bucket, open, high, low, close, volume, trade_count
		FROM %s ca2
		WHERE ca2.symbol_id = s.symbol_id
		ORDER BY ca2.bucket DESC
		LIMIT $2
	) ca ON true
	WHERE s.symbol = ANY($1)
	ORDER BY s.symbol, ca.bucket
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
