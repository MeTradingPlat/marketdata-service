package timescale

import (
	"context"
	"errors"
	"fmt"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FundamentalsRepository struct {
	pool *pgxpool.Pool
}

func NewFundamentalsRepository(pool *pgxpool.Pool) *FundamentalsRepository {
	return &FundamentalsRepository{pool: pool}
}

const getFundamentalsSQL = `
	SELECT s.is_etf, COALESCE(d.dividend_amount, 0), COALESCE(d.dividend_frequency, 0)
	FROM tracked_symbols s
	LEFT JOIN dividends d ON d.symbol_id = s.symbol_id
	WHERE s.symbol = $1
`

func (r *FundamentalsRepository) Get(ctx context.Context, symbol string) (domain.Fundamentals, error) {
	f := domain.Fundamentals{Symbol: symbol}
	err := r.pool.QueryRow(ctx, getFundamentalsSQL, symbol).Scan(&f.IsEtf, &f.DividendAmount, &f.DividendFrequency)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Fundamentals{}, fmt.Errorf("symbol %s not tracked", symbol)
	}
	if err != nil {
		return domain.Fundamentals{}, fmt.Errorf("querying fundamentals for %s: %w", symbol, err)
	}
	return f, nil
}

const upsertDividendSQL = `
	INSERT INTO dividends (symbol_id, dividend_amount, dividend_frequency, updated_at)
	SELECT symbol_id, $2, $3, now() FROM tracked_symbols WHERE symbol = $1
	ON CONFLICT (symbol_id) DO UPDATE SET
		dividend_amount = EXCLUDED.dividend_amount,
		dividend_frequency = EXCLUDED.dividend_frequency,
		updated_at = EXCLUDED.updated_at
`

func (r *FundamentalsRepository) UpsertDividends(ctx context.Context, fundamentals []domain.Fundamentals) error {
	if len(fundamentals) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, f := range fundamentals {
		batch.Queue(upsertDividendSQL, f.Symbol, f.DividendAmount, f.DividendFrequency)
	}
	results := r.pool.SendBatch(ctx, batch)
	defer results.Close()
	for range fundamentals {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("upserting dividend batch: %w", err)
		}
	}
	return nil
}
