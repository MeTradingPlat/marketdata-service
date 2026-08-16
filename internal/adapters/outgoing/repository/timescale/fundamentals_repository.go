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

const fundamentalsSelectSQL = `
	s.is_etf,
	COALESCE(d.dividend_amount, 0), COALESCE(d.dividend_frequency, 0),
	COALESCE(d.trading_status, ''), COALESCE(d.halt_start_time, -1), COALESCE(d.halt_end_time, -1),
	d.market_data_updated_at,
	COALESCE(d.market_cap, 0), COALESCE(d.eps, 0), COALESCE(d.beta, 0),
	COALESCE(d.lendability, ''), COALESCE(d.borrow_rate, 0),
	COALESCE(d.liquidity, 0), COALESCE(d.liquidity_rating, 0),
	COALESCE(d.implied_volatility_index, 0), COALESCE(d.implied_volatility_rank, 0),
	COALESCE(d.implied_volatility_percentile, 0), COALESCE(d.next_earnings_date, ''),
	d.metrics_updated_at,
	d.shares_outstanding, d.float_shares, d.short_interest, d.short_ratio, d.external_updated_at
`

const getFundamentalsSQL = `
	SELECT ` + fundamentalsSelectSQL + `
	FROM tracked_symbols s
	LEFT JOIN dividends d ON d.symbol_id = s.symbol_id
	WHERE s.symbol = $1
`

func scanFundamentals(row pgx.Row, f *domain.Fundamentals) error {
	return row.Scan(
		&f.IsEtf, &f.DividendAmount, &f.DividendFrequency,
		&f.TradingStatus, &f.HaltStartTime, &f.HaltEndTime, &f.MarketDataUpdatedAt,
		&f.MarketCap, &f.Eps, &f.Beta,
		&f.Lendability, &f.BorrowRate,
		&f.Liquidity, &f.LiquidityRating,
		&f.ImpliedVolatilityIndex, &f.ImpliedVolatilityRank,
		&f.ImpliedVolatilityPercentile, &f.NextEarningsDate, &f.MetricsUpdatedAt,
		&f.SharesOutstanding, &f.FloatShares, &f.ShortInterest, &f.ShortRatio, &f.ExternalUpdatedAt,
	)
}

func (r *FundamentalsRepository) Get(ctx context.Context, symbol string) (domain.Fundamentals, error) {
	f := domain.Fundamentals{Symbol: symbol}
	err := scanFundamentals(r.pool.QueryRow(ctx, getFundamentalsSQL, symbol), &f)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Fundamentals{}, fmt.Errorf("symbol %s not tracked", symbol)
	}
	if err != nil {
		return domain.Fundamentals{}, fmt.Errorf("querying fundamentals for %s: %w", symbol, err)
	}
	return f, nil
}

const getFundamentalsBatchSQL = `
	SELECT s.symbol, ` + fundamentalsSelectSQL + `
	FROM tracked_symbols s
	LEFT JOIN dividends d ON d.symbol_id = s.symbol_id
	WHERE s.symbol = ANY($1)
`

// GetBatch es una sola consulta acotada por symbol = ANY($1) en vez de N
// consultas individuales -- pensada para /marketdata/fundamentals/realtime,
// que puede pedir miles de simbolos de una (signal-processing-service).
func (r *FundamentalsRepository) GetBatch(ctx context.Context, symbols []string) (map[string]domain.Fundamentals, error) {
	if len(symbols) == 0 {
		return map[string]domain.Fundamentals{}, nil
	}
	rows, err := r.pool.Query(ctx, getFundamentalsBatchSQL, symbols)
	if err != nil {
		return nil, fmt.Errorf("querying fundamentals batch: %w", err)
	}
	defer rows.Close()

	result := make(map[string]domain.Fundamentals, len(symbols))
	for rows.Next() {
		var symbol string
		f := domain.Fundamentals{}
		if err := rows.Scan(&symbol, &f.IsEtf, &f.DividendAmount, &f.DividendFrequency,
			&f.TradingStatus, &f.HaltStartTime, &f.HaltEndTime, &f.MarketDataUpdatedAt,
			&f.MarketCap, &f.Eps, &f.Beta,
			&f.Lendability, &f.BorrowRate,
			&f.Liquidity, &f.LiquidityRating,
			&f.ImpliedVolatilityIndex, &f.ImpliedVolatilityRank,
			&f.ImpliedVolatilityPercentile, &f.NextEarningsDate, &f.MetricsUpdatedAt,
			&f.SharesOutstanding, &f.FloatShares, &f.ShortInterest, &f.ShortRatio, &f.ExternalUpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning fundamentals batch row: %w", err)
		}
		f.Symbol = symbol
		result[symbol] = f
	}
	return result, rows.Err()
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

// upsertExternalFundamentalsSQL cubre shares_outstanding/float_shares
// (SEC EDGAR) y short_interest/short_ratio (FINRA) -- fuentes externas a
// TastyTrade, refrescadas juntas en el barrido nocturno pero con su propio
// external_updated_at para distinguir "nunca corrio" de "corrio y no hubo
// dato" en ese simbolo puntual.
const upsertExternalFundamentalsSQL = `
	INSERT INTO dividends (symbol_id, shares_outstanding, float_shares, short_interest, short_ratio, external_updated_at)
	SELECT symbol_id, $2, $3, $4, $5, now() FROM tracked_symbols WHERE symbol = $1
	ON CONFLICT (symbol_id) DO UPDATE SET
		shares_outstanding = EXCLUDED.shares_outstanding,
		float_shares = EXCLUDED.float_shares,
		short_interest = EXCLUDED.short_interest,
		short_ratio = EXCLUDED.short_ratio,
		external_updated_at = EXCLUDED.external_updated_at
`

func (r *FundamentalsRepository) UpsertExternalFundamentals(ctx context.Context, fundamentals []domain.Fundamentals) error {
	if len(fundamentals) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, f := range fundamentals {
		batch.Queue(upsertExternalFundamentalsSQL, f.Symbol, f.SharesOutstanding, f.FloatShares, f.ShortInterest, f.ShortRatio)
	}
	results := r.pool.SendBatch(ctx, batch)
	defer results.Close()
	for range fundamentals {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("upserting external fundamentals batch: %w", err)
		}
	}
	return nil
}
