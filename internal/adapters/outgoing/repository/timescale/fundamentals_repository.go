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
	SELECT s.is_etf,
		COALESCE(d.dividend_amount, 0), COALESCE(d.dividend_frequency, 0),
		COALESCE(d.trading_status, ''), COALESCE(d.halt_start_time, -1), COALESCE(d.halt_end_time, -1),
		COALESCE(d.market_cap, 0), COALESCE(d.eps, 0), COALESCE(d.beta, 0),
		COALESCE(d.lendability, ''), COALESCE(d.borrow_rate, 0),
		COALESCE(d.liquidity, 0), COALESCE(d.liquidity_rating, 0),
		COALESCE(d.implied_volatility_index, 0), COALESCE(d.implied_volatility_rank, 0),
		COALESCE(d.implied_volatility_percentile, 0), COALESCE(d.next_earnings_date, '')
	FROM tracked_symbols s
	LEFT JOIN dividends d ON d.symbol_id = s.symbol_id
	WHERE s.symbol = $1
`

func (r *FundamentalsRepository) Get(ctx context.Context, symbol string) (domain.Fundamentals, error) {
	f := domain.Fundamentals{Symbol: symbol}
	err := r.pool.QueryRow(ctx, getFundamentalsSQL, symbol).Scan(
		&f.IsEtf, &f.DividendAmount, &f.DividendFrequency,
		&f.TradingStatus, &f.HaltStartTime, &f.HaltEndTime,
		&f.MarketCap, &f.Eps, &f.Beta,
		&f.Lendability, &f.BorrowRate,
		&f.Liquidity, &f.LiquidityRating,
		&f.ImpliedVolatilityIndex, &f.ImpliedVolatilityRank,
		&f.ImpliedVolatilityPercentile, &f.NextEarningsDate,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Fundamentals{}, fmt.Errorf("symbol %s not tracked", symbol)
	}
	if err != nil {
		return domain.Fundamentals{}, fmt.Errorf("querying fundamentals for %s: %w", symbol, err)
	}
	return f, nil
}

// upsertDividendSQL cubre los campos que salen de /market-data/by-type
// (DividendInfo) -- dividendos + halt/trading-status, la misma llamada REST
// trae ambos. Solo toca sus propias columnas, para no pisar lo que haya
// puesto UpsertMarketMetrics con datos de otro endpoint.
const upsertDividendSQL = `
	INSERT INTO dividends (symbol_id, dividend_amount, dividend_frequency, trading_status, halt_start_time, halt_end_time, updated_at, market_data_updated_at)
	SELECT symbol_id, $2, $3, $4, $5, $6, now(), now() FROM tracked_symbols WHERE symbol = $1
	ON CONFLICT (symbol_id) DO UPDATE SET
		dividend_amount = EXCLUDED.dividend_amount,
		dividend_frequency = EXCLUDED.dividend_frequency,
		trading_status = EXCLUDED.trading_status,
		halt_start_time = EXCLUDED.halt_start_time,
		halt_end_time = EXCLUDED.halt_end_time,
		updated_at = EXCLUDED.updated_at,
		market_data_updated_at = EXCLUDED.market_data_updated_at
`

func (r *FundamentalsRepository) UpsertDividends(ctx context.Context, fundamentals []domain.Fundamentals) error {
	if len(fundamentals) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, f := range fundamentals {
		batch.Queue(upsertDividendSQL, f.Symbol, f.DividendAmount, f.DividendFrequency, f.TradingStatus, f.HaltStartTime, f.HaltEndTime)
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

// upsertMarketMetricsSQL cubre los campos que salen de /market-metrics --
// endpoint separado de DividendInfo, se llama y refresca aparte. Solo toca
// sus propias columnas, mismo motivo que upsertDividendSQL.
const upsertMarketMetricsSQL = `
	INSERT INTO dividends (symbol_id, market_cap, eps, beta, lendability, borrow_rate, liquidity, liquidity_rating,
		implied_volatility_index, implied_volatility_rank, implied_volatility_percentile, next_earnings_date, metrics_updated_at)
	SELECT symbol_id, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, now() FROM tracked_symbols WHERE symbol = $1
	ON CONFLICT (symbol_id) DO UPDATE SET
		market_cap = EXCLUDED.market_cap,
		eps = EXCLUDED.eps,
		beta = EXCLUDED.beta,
		lendability = EXCLUDED.lendability,
		borrow_rate = EXCLUDED.borrow_rate,
		liquidity = EXCLUDED.liquidity,
		liquidity_rating = EXCLUDED.liquidity_rating,
		implied_volatility_index = EXCLUDED.implied_volatility_index,
		implied_volatility_rank = EXCLUDED.implied_volatility_rank,
		implied_volatility_percentile = EXCLUDED.implied_volatility_percentile,
		next_earnings_date = EXCLUDED.next_earnings_date,
		metrics_updated_at = EXCLUDED.metrics_updated_at
`

func (r *FundamentalsRepository) UpsertMarketMetrics(ctx context.Context, fundamentals []domain.Fundamentals) error {
	if len(fundamentals) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, f := range fundamentals {
		batch.Queue(upsertMarketMetricsSQL, f.Symbol, f.MarketCap, f.Eps, f.Beta, f.Lendability, f.BorrowRate,
			f.Liquidity, f.LiquidityRating, f.ImpliedVolatilityIndex, f.ImpliedVolatilityRank,
			f.ImpliedVolatilityPercentile, f.NextEarningsDate)
	}
	results := r.pool.SendBatch(ctx, batch)
	defer results.Close()
	for range fundamentals {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("upserting market metrics batch: %w", err)
		}
	}
	return nil
}
