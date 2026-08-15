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
	INSERT INTO tracked_symbols (symbol, market, is_etf) VALUES ($1, $2, $3)
	ON CONFLICT (symbol) DO UPDATE SET market = EXCLUDED.market, is_active = TRUE, is_etf = EXCLUDED.is_etf
`

func (r *SymbolRepository) Upsert(ctx context.Context, symbols []domain.Symbol) error {
	if len(symbols) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, s := range symbols {
		batch.Queue(upsertSymbolSQL, s.Symbol, s.Market, s.IsEtf)
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

const deactivateSymbolsSQL = `UPDATE tracked_symbols SET is_active = FALSE WHERE symbol = ANY($1)`

// Deactivate no borra ninguna vela ya guardada -- solo saca al simbolo de
// Tracked() para que el catch-up diario deje de pedirle mas historia, en
// caso de que TastyTrade ya no lo reporte como activo.
func (r *SymbolRepository) Deactivate(ctx context.Context, symbols []string) error {
	if len(symbols) == 0 {
		return nil
	}
	if _, err := r.pool.Exec(ctx, deactivateSymbolsSQL, symbols); err != nil {
		return fmt.Errorf("deactivating symbols: %w", err)
	}
	return nil
}
