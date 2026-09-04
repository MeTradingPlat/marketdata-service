package timescale

import (
	"context"
	"fmt"
	"time"
)

// previousPostMarketVolumeWindowSQL suma el volumen M1 de la sesion post-
// market [16:00 ET, medianoche ET) de UN dia especifico -- misma frontera
// que usa GetIntradaySessionsBatch para "hoy" (ver queryIntradaySessionsBatch),
// aca aplicada al dia de AYER. any_rows distingue "el simbolo no opero ese
// dia" (0 filas, hay que seguir buscando dias atras) de "opero pero sin
// volumen post-market real" (post_vol=0 es un dato valido, no ausencia).
const previousPostMarketVolumeWindowSQL = `
	SELECT s.symbol, COALESCE(SUM(c.volume) FILTER (WHERE c.ts >= $2), 0), COUNT(*)
	FROM candles c JOIN tracked_symbols s ON s.symbol_id = c.symbol_id
	WHERE s.symbol = ANY($1) AND c.timeframe = 'M1' AND c.ts >= $3 AND c.ts < $4
	GROUP BY s.symbol
`

// GetPreviousPostMarketVolumeBatch: mismo patron dia-por-dia que
// GetPreviousSessionCloseBatch (ver ese comentario) -- camina hacia atras
// hasta prevSessionCloseDays buscando el ultimo dia habil con datos M1 para
// cada simbolo, y suma su volumen post-market. Usado por
// RefreshPrevPostMarketVolume para que un escaner en premarket (donde el
// postMarketVolume de HOY todavia no existe, ver domain.Fundamentals.
// PrevPostMarketVolume) tenga un dato real de la sesion anterior.
func (r *CandleRepository) GetPreviousPostMarketVolumeBatch(ctx context.Context, symbols []string, before time.Time) (map[string]int64, error) {
	result := make(map[string]int64, len(symbols))
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
		marketClose := day.Add(16 * time.Hour)
		dayEnd := day.Add(24 * time.Hour)

		resolved := make(map[string]struct{}, len(pending))
		for _, batch := range chunkSymbols(pending, universeBatchChunkSize) {
			if err := r.queryPreviousPostMarketVolumeWindow(ctx, batch, marketClose, day, dayEnd, result, resolved); err != nil {
				return nil, fmt.Errorf("querying previous post market volume window (day -%d): %w", d, err)
			}
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

func (r *CandleRepository) queryPreviousPostMarketVolumeWindow(ctx context.Context, batch []string, marketClose, dayStart, dayEnd time.Time, result map[string]int64, resolved map[string]struct{}) error {
	rows, err := r.snapshotPool.Query(ctx, previousPostMarketVolumeWindowSQL, batch, marketClose, dayStart, dayEnd)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var symbol string
		var postVolume int64
		var anyRows int
		if err := rows.Scan(&symbol, &postVolume, &anyRows); err != nil {
			return fmt.Errorf("scanning previous post market volume window row: %w", err)
		}
		if anyRows == 0 {
			continue
		}
		result[symbol] = postVolume
		resolved[symbol] = struct{}{}
	}
	return rows.Err()
}
