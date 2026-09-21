package timescale

import (
	"context"
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type VolumeProfileRepository struct {
	pool *pgxpool.Pool
}

func NewVolumeProfileRepository(pool *pgxpool.Pool) *VolumeProfileRepository {
	return &VolumeProfileRepository{pool: pool}
}

const staleVolumeProfilesSQL = `
	SELECT s.symbol
	FROM tracked_symbols s
	LEFT JOIN volume_profiles p ON p.symbol_id = s.symbol_id
	WHERE s.is_active = TRUE AND (p.computed_at IS NULL OR p.computed_at < $1)
	ORDER BY s.symbol`

func (r *VolumeProfileRepository) GetSymbolsWithStaleVolumeProfile(ctx context.Context, windowStart time.Time) ([]string, error) {
	rows, err := r.pool.Query(ctx, staleVolumeProfilesSQL, windowStart)
	if err != nil {
		return nil, fmt.Errorf("listing symbols with stale volume profile: %w", err)
	}
	defer rows.Close()
	var symbols []string
	for rows.Next() {
		var symbol string
		if err := rows.Scan(&symbol); err != nil {
			return nil, fmt.Errorf("scanning stale volume profile symbol: %w", err)
		}
		symbols = append(symbols, symbol)
	}
	return symbols, rows.Err()
}

const upsertVolumeProfileSQL = `
	INSERT INTO volume_profiles (symbol_id, sessions, cumulative, computed_at)
	SELECT symbol_id, $2, $3, now() FROM tracked_symbols WHERE symbol = $1
	ON CONFLICT (symbol_id) DO UPDATE
	SET sessions = EXCLUDED.sessions, cumulative = EXCLUDED.cumulative, computed_at = now()`

func (r *VolumeProfileRepository) SaveBatch(ctx context.Context, profiles map[string]domain.VolumeProfile, attemptedOnly []string) error {
	total := len(profiles) + len(attemptedOnly)
	if total == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for symbol, profile := range profiles {
		batch.Queue(upsertVolumeProfileSQL, symbol, int16(profile.Sessions), profile.Cumulative)
	}
	for _, symbol := range attemptedOnly {
		batch.Queue(upsertVolumeProfileSQL, symbol, int16(0), []float32{})
	}
	results := r.pool.SendBatch(ctx, batch)
	defer results.Close()
	for i := 0; i < total; i++ {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("upserting volume profile batch: %w", err)
		}
	}
	return nil
}

const getVolumeProfilesSQL = `
	SELECT s.symbol, p.sessions, p.cumulative
	FROM volume_profiles p JOIN tracked_symbols s ON s.symbol_id = p.symbol_id
	WHERE s.symbol = ANY($1) AND p.sessions > 0`

func (r *VolumeProfileRepository) GetBatch(ctx context.Context, symbols []string) (map[string]domain.VolumeProfile, error) {
	rows, err := r.pool.Query(ctx, getVolumeProfilesSQL, symbols)
	if err != nil {
		return nil, fmt.Errorf("loading volume profiles for %d symbols: %w", len(symbols), err)
	}
	defer rows.Close()
	result := make(map[string]domain.VolumeProfile, len(symbols))
	for rows.Next() {
		p := domain.VolumeProfile{}
		var sessions int16
		if err := rows.Scan(&p.Symbol, &sessions, &p.Cumulative); err != nil {
			return nil, fmt.Errorf("scanning volume profile: %w", err)
		}
		p.Sessions = int(sessions)
		result[p.Symbol] = p
	}
	return result, rows.Err()
}
