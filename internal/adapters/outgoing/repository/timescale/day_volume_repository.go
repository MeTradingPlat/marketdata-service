package timescale

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DayVolumeRepository struct {
	pool *pgxpool.Pool
}

func NewDayVolumeRepository(pool *pgxpool.Pool) *DayVolumeRepository {
	return &DayVolumeRepository{pool: pool}
}

const upsertDayVolumeSQL = `
	INSERT INTO day_volumes (symbol_id, day, volume, updated_at)
	SELECT symbol_id, $2, $3, now() FROM tracked_symbols WHERE symbol = $1
	ON CONFLICT (symbol_id) DO UPDATE
	SET day = EXCLUDED.day, volume = EXCLUDED.volume, updated_at = now()`

// SaveBatch pisa el volumen de cada simbolo del lote -- un simbolo sin dato
// esta ronda (illiquido, sin trades hoy) simplemente no se toca, conserva
// lo que ya tenia (mismo criterio que VolumeProfileRepository.SaveBatch).
func (r *DayVolumeRepository) SaveBatch(ctx context.Context, day time.Time, volumes map[string]int64) error {
	if len(volumes) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for symbol, volume := range volumes {
		batch.Queue(upsertDayVolumeSQL, symbol, day, volume)
	}
	results := r.pool.SendBatch(ctx, batch)
	defer results.Close()
	for range volumes {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("upserting day volume batch: %w", err)
		}
	}
	return nil
}

const getDayVolumesSQL = `
	SELECT s.symbol, v.volume
	FROM day_volumes v JOIN tracked_symbols s ON s.symbol_id = v.symbol_id
	WHERE s.symbol = ANY($1) AND v.day = $2`

// GetBatch solo devuelve simbolos con dato de HOY -- un valor de ayer que
// no se refresco todavia esta mañana no debe hacerse pasar por el de hoy
// (mismo motivo que day en la tabla).
func (r *DayVolumeRepository) GetBatch(ctx context.Context, symbols []string, day time.Time) (map[string]int64, error) {
	rows, err := r.pool.Query(ctx, getDayVolumesSQL, symbols, day)
	if err != nil {
		return nil, fmt.Errorf("loading day volumes for %d symbols: %w", len(symbols), err)
	}
	defer rows.Close()
	result := make(map[string]int64, len(symbols))
	for rows.Next() {
		var symbol string
		var volume int64
		if err := rows.Scan(&symbol, &volume); err != nil {
			return nil, fmt.Errorf("scanning day volume: %w", err)
		}
		result[symbol] = volume
	}
	return result, rows.Err()
}
