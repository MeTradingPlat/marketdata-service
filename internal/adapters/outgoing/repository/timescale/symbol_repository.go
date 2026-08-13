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
	INSERT INTO tracked_symbols (symbol, market) VALUES ($1, $2)
	ON CONFLICT (symbol) DO UPDATE SET market = EXCLUDED.market, is_active = TRUE
`

func (r *SymbolRepository) Upsert(ctx context.Context, symbols []domain.Symbol) error {
	if len(symbols) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, s := range symbols {
		batch.Queue(upsertSymbolSQL, s.Symbol, s.Market)
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

const trackedSymbolsSQL = `SELECT symbol, market FROM tracked_symbols WHERE is_active = TRUE`

func (r *SymbolRepository) Tracked(ctx context.Context) ([]domain.Symbol, error) {
	rows, err := r.pool.Query(ctx, trackedSymbolsSQL)
	if err != nil {
		return nil, fmt.Errorf("querying tracked symbols: %w", err)
	}
	defer rows.Close()

	var symbols []domain.Symbol
	for rows.Next() {
		var s domain.Symbol
		if err := rows.Scan(&s.Symbol, &s.Market); err != nil {
			return nil, fmt.Errorf("scanning symbol row: %w", err)
		}
		symbols = append(symbols, s)
	}
	return symbols, rows.Err()
}
