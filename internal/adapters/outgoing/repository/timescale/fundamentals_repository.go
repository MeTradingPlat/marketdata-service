package timescale

import (
	"context"
	"errors"
	"fmt"
	"time"

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
	d.shares_outstanding, d.float_shares, d.short_interest, d.short_ratio,
	d.short_interest_shares, COALESCE(d.short_interest_settlement, ''),
	d.external_updated_at, d.float_updated_at,
	COALESCE(d.occurred_date, '')
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
	for _, f := range fundamentals {
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
	for _, f := range fundamentals {
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

// getSymbolsWithStaleEarningsSQL: un simbolo esta "debido" si nunca se
// busco (next_earnings_date NULL/vacio) o si la fecha que tenemos ya paso
// -- el endpoint de historic-corporate-events es por-simbolo (sin batch),
// asi que acotar a los que de verdad lo necesitan evita pedirle esto a
// TastyTrade para el universo entero cada noche cuando earnings solo
// cambia ~4 veces al año por emisor. Los simbolos con "/" (warrants y
// units, ej. MTAL/WS) se excluyen de entrada: la barra parte la URL del
// endpoint (404 garantizado, confirmado en vivo) y esas clases de activo
// no reportan earnings de todas formas.
const getSymbolsWithStaleEarningsSQL = `
	SELECT s.symbol
	FROM tracked_symbols s
	LEFT JOIN dividends d ON d.symbol_id = s.symbol_id
	WHERE s.is_active = TRUE
	AND s.symbol !~ '/'
	AND (
		d.next_earnings_date IS NULL OR d.next_earnings_date = ''
		OR (d.next_earnings_date ~ '^\d{4}-\d{2}-\d{2}$' AND d.next_earnings_date::date <= CURRENT_DATE)
	)
`

func (r *FundamentalsRepository) GetSymbolsWithStaleEarnings(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, getSymbolsWithStaleEarningsSQL)
	if err != nil {
		return nil, fmt.Errorf("querying symbols with stale earnings: %w", err)
	}
	defer rows.Close()

	var symbols []string
	for rows.Next() {
		var symbol string
		if err := rows.Scan(&symbol); err != nil {
			return nil, fmt.Errorf("scanning stale earnings symbol row: %w", err)
		}
		symbols = append(symbols, symbol)
	}
	return symbols, rows.Err()
}

// upsertBetaSQL solo toca beta -- el beta calculado con velas propias
// (5Y monthly, ver domain.MonthlyBeta) es mejor que el de TastyTrade
// (ventana corta, da valores raros en ADRs), pero solo se manda para los
// simbolos donde se pudo calcular; el resto conserva el de TastyTrade.
const upsertBetaSQL = `
	INSERT INTO dividends (symbol_id, beta)
	SELECT symbol_id, $2 FROM tracked_symbols WHERE symbol = $1
	ON CONFLICT (symbol_id) DO UPDATE SET beta = EXCLUDED.beta
`

func (r *FundamentalsRepository) UpsertBeta(ctx context.Context, fundamentals []domain.Fundamentals) error {
	if len(fundamentals) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, f := range fundamentals {
		batch.Queue(upsertBetaSQL, f.Symbol, f.Beta)
	}
	results := r.pool.SendBatch(ctx, batch)
	defer results.Close()
	for range fundamentals {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("upserting beta batch: %w", err)
		}
	}
	return nil
}

// getSymbolsDueForFloatRefreshSQL trae el lote de simbolos con
// sharesOutstanding+insiderShares ya conocidos (condicion para poder
// calcular floatShares real) ordenados por float_updated_at ascendente --
// NULLS FIRST prioriza los que todavia solo tienen el heuristico del 90%,
// nunca refrescados a real. Ver RefreshBeneficialOwners.
const getSymbolsDueForFloatRefreshSQL = `
	SELECT s.symbol, d.shares_outstanding, d.insider_shares, d.insider_ciks
	FROM tracked_symbols s
	JOIN dividends d ON d.symbol_id = s.symbol_id
	WHERE s.is_active = TRUE AND d.shares_outstanding IS NOT NULL AND d.insider_shares IS NOT NULL
	ORDER BY d.float_updated_at ASC NULLS FIRST
	LIMIT $1
`

// RecordStepDone registra cuando termino un refresh de fundamentales -- la
// marca persistida que evita recalcular en cada reinicio del contenedor: un
// paso se salta si su done_at cae dentro de la ventana de mantenimiento
// actual (ver LastMaintenanceWindowStart en daily_catchup.go). Solo se
// registra al terminar OK; un refresh fallido queda stale y se reintenta.
func (r *FundamentalsRepository) RecordStepDone(ctx context.Context, step string, at time.Time) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO fundamental_refresh_log (step, done_at) VALUES ($1, $2)
		 ON CONFLICT (step) DO UPDATE SET done_at = EXCLUDED.done_at`,
		step, at.UTC())
	if err != nil {
		return fmt.Errorf("recording fundamental refresh %s: %w", step, err)
	}
	return nil
}

// StepDoneAt devuelve cuando se calculo por ultima vez el paso (false si
// nunca se calculo -- primer arranque o paso que nunca corrio).
func (r *FundamentalsRepository) StepDoneAt(ctx context.Context, step string) (time.Time, bool, error) {
	var at time.Time
	err := r.pool.QueryRow(ctx,
		`SELECT done_at FROM fundamental_refresh_log WHERE step = $1`, step).Scan(&at)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("reading fundamental refresh %s: %w", step, err)
	}
	return at, true, nil
}

func (r *FundamentalsRepository) GetSymbolsDueForFloatRefresh(ctx context.Context, limit int) ([]domain.Fundamentals, error) {
	rows, err := r.pool.Query(ctx, getSymbolsDueForFloatRefreshSQL, limit)
	if err != nil {
		return nil, fmt.Errorf("querying symbols due for float refresh: %w", err)
	}
	defer rows.Close()

	var result []domain.Fundamentals
	for rows.Next() {
		f := domain.Fundamentals{}
		if err := rows.Scan(&f.Symbol, &f.SharesOutstanding, &f.InsiderShares, &f.InsiderCiks); err != nil {
			return nil, fmt.Errorf("scanning float refresh candidate row: %w", err)
		}
		result = append(result, f)
	}
	return result, rows.Err()
}
