package timescale

import (
	"context"
	"fmt"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/jackc/pgx/v5"
)

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
	for _, f := range bySymbol(fundamentals) {
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
	for _, f := range bySymbol(fundamentals) {
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

// upsertExternalFundamentalsSQL cubre shares_outstanding/float_shares (SEC
// EDGAR), short_interest/short_ratio (FINRA), e insider_shares/insider_ciks
// (SEC EDGAR insider bulk) -- tres fuentes independientes que llaman a este
// mismo upsert cada una con su propio subconjunto de columnas lleno. Cada
// columna usa COALESCE contra el valor ya guardado (no EXCLUDED a secas):
// sin esto, RefreshBeneficialOwners (que solo llena floatShares) borraria
// el shortInterest que puso FINRA la noche anterior, y viceversa.
// external_updated_at si se pisa siempre -- representa "algo externo se
// refresco", no un campo puntual.
const upsertExternalFundamentalsSQL = `
	INSERT INTO dividends (symbol_id, shares_outstanding, float_shares, short_interest, short_ratio, short_interest_shares, short_interest_settlement, insider_shares, insider_ciks, float_updated_at, external_updated_at)
	SELECT symbol_id, $2, $3, $4, $5, $6, $7, $8, $9, $10, now() FROM tracked_symbols WHERE symbol = $1
	ON CONFLICT (symbol_id) DO UPDATE SET
		shares_outstanding = COALESCE(EXCLUDED.shares_outstanding, dividends.shares_outstanding),
		float_shares = COALESCE(EXCLUDED.float_shares, dividends.float_shares),
		short_interest = COALESCE(EXCLUDED.short_interest, dividends.short_interest),
		short_ratio = COALESCE(EXCLUDED.short_ratio, dividends.short_ratio),
		short_interest_shares = COALESCE(EXCLUDED.short_interest_shares, dividends.short_interest_shares),
		short_interest_settlement = COALESCE(EXCLUDED.short_interest_settlement, dividends.short_interest_settlement),
		insider_shares = COALESCE(EXCLUDED.insider_shares, dividends.insider_shares),
		insider_ciks = COALESCE(EXCLUDED.insider_ciks, dividends.insider_ciks),
		float_updated_at = COALESCE(EXCLUDED.float_updated_at, dividends.float_updated_at),
		external_updated_at = EXCLUDED.external_updated_at
`

func (r *FundamentalsRepository) UpsertExternalFundamentals(ctx context.Context, fundamentals []domain.Fundamentals) error {
	if len(fundamentals) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, f := range bySymbol(fundamentals) {
		batch.Queue(upsertExternalFundamentalsSQL, f.Symbol, f.SharesOutstanding, f.FloatShares, f.ShortInterest, f.ShortRatio,
			f.ShortInterestShares, f.ShortInterestSettlement, f.InsiderShares, f.InsiderCiks, f.FloatUpdatedAt)
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

// upsertEarningsHistorySQL cubre occurred_date (ultimo reporte real) y,
// solo cuando hay una prediccion valida, next_earnings_date -- NULLIF
// convierte "" (nuestro "no hay dato" para estos dos campos de texto, ver
// domain.Fundamentals) en SQL NULL antes del COALESCE, para no pisar un
// next_earnings_date que MarketMetrics ya haya puesto esa misma noche.
const upsertEarningsHistorySQL = `
	INSERT INTO dividends (symbol_id, occurred_date, next_earnings_date, earnings_updated_at)
	SELECT symbol_id, NULLIF($2, ''), NULLIF($3, ''), now() FROM tracked_symbols WHERE symbol = $1
	ON CONFLICT (symbol_id) DO UPDATE SET
		occurred_date = COALESCE(EXCLUDED.occurred_date, dividends.occurred_date),
		next_earnings_date = COALESCE(EXCLUDED.next_earnings_date, dividends.next_earnings_date),
		earnings_updated_at = EXCLUDED.earnings_updated_at
`

func (r *FundamentalsRepository) UpsertEarningsHistory(ctx context.Context, fundamentals []domain.Fundamentals) error {
	if len(fundamentals) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, f := range bySymbol(fundamentals) {
		batch.Queue(upsertEarningsHistorySQL, f.Symbol, f.OccurredDate, f.NextEarningsDate)
	}
	results := r.pool.SendBatch(ctx, batch)
	defer results.Close()
	for range fundamentals {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("upserting earnings history batch: %w", err)
		}
	}
	return nil
}
