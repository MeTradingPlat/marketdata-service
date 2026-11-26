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
	COALESCE(d.market_cap, 0), d.eps, COALESCE(d.beta, 0),
	COALESCE(d.lendability, ''), COALESCE(d.borrow_rate, 0),
	COALESCE(d.liquidity, 0), COALESCE(d.liquidity_rating, 0),
	COALESCE(d.implied_volatility_index, 0), COALESCE(d.implied_volatility_rank, 0),
	COALESCE(d.implied_volatility_percentile, 0), COALESCE(d.next_earnings_date, ''),
	d.metrics_updated_at,
	d.shares_outstanding, d.float_shares, d.short_interest, d.short_ratio,
	d.short_interest_shares, COALESCE(d.short_interest_settlement, ''),
	d.external_updated_at, d.float_updated_at,
	COALESCE(d.occurred_date, ''),
	d.prev_close, d.prev_close_updated_at,
	d.prev_post_market_volume, d.prev_post_market_volume_updated_at,
	d.open_interest, d.open_interest_updated_at
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
		&f.SharesOutstanding, &f.FloatShares, &f.ShortInterest, &f.ShortRatio,
		&f.ShortInterestShares, &f.ShortInterestSettlement,
		&f.ExternalUpdatedAt, &f.FloatUpdatedAt,
		&f.OccurredDate,
		&f.PrevClose, &f.PrevCloseUpdatedAt,
		&f.PrevPostMarketVolume, &f.PrevPostMarketVolumeUpdatedAt,
		&f.OpenInterest, &f.OpenInterestUpdatedAt,
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
			&f.SharesOutstanding, &f.FloatShares, &f.ShortInterest, &f.ShortRatio,
			&f.ShortInterestShares, &f.ShortInterestSettlement,
			&f.ExternalUpdatedAt, &f.FloatUpdatedAt,
			&f.OccurredDate,
			&f.PrevClose, &f.PrevCloseUpdatedAt,
			&f.PrevPostMarketVolume, &f.PrevPostMarketVolumeUpdatedAt,
			&f.OpenInterest, &f.OpenInterestUpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning fundamentals batch row: %w", err)
		}
		f.Symbol = symbol
		result[symbol] = f
	}
	return result, rows.Err()
}
