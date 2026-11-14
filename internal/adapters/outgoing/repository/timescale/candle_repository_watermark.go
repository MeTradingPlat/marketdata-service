package timescale

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/jackc/pgx/v5"
)

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
