package timescale

import (
	"context"
	"fmt"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SymbolRepository struct {
	pool *pgxpool.Pool
}

func NewSymbolRepository(pool *pgxpool.Pool) *SymbolRepository {
	return &SymbolRepository{pool: pool}
}

const upsertSymbolSQL = `
	INSERT INTO tracked_symbols (symbol, market, is_etf, description) VALUES ($1, $2, $3, $4)
	ON CONFLICT (symbol) DO UPDATE SET
		market = EXCLUDED.market, is_active = TRUE, is_etf = EXCLUDED.is_etf, description = EXCLUDED.description
`

func (r *SymbolRepository) Upsert(ctx context.Context, symbols []domain.Symbol) error {
	if len(symbols) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, s := range symbols {
		batch.Queue(upsertSymbolSQL, s.Symbol, s.Market, s.IsEtf, s.Description)
	}
	results := r.pool.SendBatch(ctx, batch)
	defer results.Close()
	for range symbols {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("upserting symbol batch: %w", err)
		}
	}
	return nil
}

const trackedSymbolsSQL = `SELECT symbol, market, description, is_etf FROM tracked_symbols WHERE is_active = TRUE`

func (r *SymbolRepository) Tracked(ctx context.Context) ([]domain.Symbol, error) {
	rows, err := r.pool.Query(ctx, trackedSymbolsSQL)
	if err != nil {
		return nil, fmt.Errorf("querying tracked symbols: %w", err)
	}
	defer rows.Close()

	var symbols []domain.Symbol
	for rows.Next() {
		var s domain.Symbol
		if err := rows.Scan(&s.Symbol, &s.Market, &s.Description, &s.IsEtf); err != nil {
			return nil, fmt.Errorf("scanning symbol row: %w", err)
		}
		symbols = append(symbols, s)
	}
	return symbols, rows.Err()
}

const trackedSymbolsWithVolumeSQL = `SELECT symbol, market, description, is_etf, last_volume FROM tracked_symbols WHERE is_active = TRUE`

// TrackedWithVolume alimenta SymbolsCache.ReloadAll -- last_volume es el
// mismo campo que Search() ya usa para ordenar por actividad real en vez de
// alfabetico (ver Save() en candle_repository.go, que lo actualiza en cada
// vela D1 nueva).
func (r *SymbolRepository) TrackedWithVolume(ctx context.Context) ([]domain.Symbol, error) {
	rows, err := r.pool.Query(ctx, trackedSymbolsWithVolumeSQL)
	if err != nil {
		return nil, fmt.Errorf("querying tracked symbols with volume: %w", err)
	}
	defer rows.Close()

	var symbols []domain.Symbol
	for rows.Next() {
		var s domain.Symbol
		if err := rows.Scan(&s.Symbol, &s.Market, &s.Description, &s.IsEtf, &s.LastVolume); err != nil {
			return nil, fmt.Errorf("scanning symbol with volume row: %w", err)
		}
		symbols = append(symbols, s)
	}
	return symbols, rows.Err()
}
