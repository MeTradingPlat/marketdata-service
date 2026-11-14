package timescale

import (
	"context"
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

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
			FROM %[1]s c
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
		FROM %[1]s c
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

// rawBatchWindowMargin (fecha, no filas -- ver rawFetchMargin de arriba)
// mucho mas chico que batchWindowMargin=6: ese margen se calibro para el
// camino SQL con GROUP BY, donde una ventana ancha era barata porque
// Postgres reducia todo del lado del servidor antes de mandarlo. Para el
// camino crudo, una ventana ancha significa mas filas SIN reducir viajando
// de Postgres a Go, Y ademas -- confirmado en vivo el 2026-08-23 con
// EXPLAIN ANALYZE -- cruza a chunks mas viejos (ya comprimidos) del
// hypertable, forzando un Sort + Merge Append por simbolo en vez del Index
// Scan directo de un solo chunk: M15 con margen=6 (~10 dias de ventana)
// tardaba 541ms para 700 simbolos contra 66ms de M5 con margen=2 (~3.3
// dias, un solo chunk). Con margen=2 el batch entero (700 simbolos, 12
// lotes) paso de 33.35s a bajar al rango de M5.
const rawBatchWindowMargin = 2

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

	if sourcePeriod, ok := sourcePeriodOf(source); ok {
		since := time.Now().Add(-time.Duration(bars*rawBatchWindowMargin+60) * approxPeriod)
		return r.getSeriesAggregatedBatchRaw(ctx, symbols, timeframe, source, approxPeriod, sourcePeriod, bars, since)
	}

	since := time.Now().Add(-time.Duration(bars*batchWindowMargin+60) * approxPeriod)
	query := fmt.Sprintf(seriesAggregatedBatchSQL, candleTable(source))
	rows, err := r.pool.Query(ctx, query, symbols, bars, string(source), bucket, since)
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

	query := fmt.Sprintf(seriesRawBatchSQL, candleTable(source))
	rows, err := r.pool.Query(ctx, query, symbols, rawLimit, string(source), since)
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
