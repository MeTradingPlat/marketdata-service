package timescale

import (
	"context"
	"fmt"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/jackc/pgx/v5"
)

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
	INSERT INTO dividends (symbol_id, beta, beta_updated_at)
	SELECT symbol_id, $2, now() FROM tracked_symbols WHERE symbol = $1
	ON CONFLICT (symbol_id) DO UPDATE SET beta = EXCLUDED.beta, beta_updated_at = now()
`

func (r *FundamentalsRepository) UpsertBeta(ctx context.Context, fundamentals []domain.Fundamentals) error {
	if len(fundamentals) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, f := range bySymbol(fundamentals) {
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
	WHERE s.is_active = TRUE
	  AND d.shares_outstanding IS NOT NULL
	  AND d.insider_shares IS NOT NULL
	ORDER BY d.float_updated_at ASC NULLS FIRST
	LIMIT $1
`

func (r *FundamentalsRepository) GetSymbolsDueForFloatRefresh(ctx context.Context, limit int) ([]domain.Fundamentals, error) {
	rows, err := r.pool.Query(ctx, getSymbolsDueForFloatRefreshSQL, limit)
	if err != nil {
		return nil, fmt.Errorf("querying symbols due for float refresh: %w", err)
	}
	defer rows.Close()

	var result []domain.Fundamentals
	for rows.Next() {
		var f domain.Fundamentals
		if err := rows.Scan(&f.Symbol, &f.SharesOutstanding, &f.InsiderShares, &f.InsiderCiks); err != nil {
			return nil, fmt.Errorf("scanning float refresh symbol row: %w", err)
		}
		result = append(result, f)
	}
	return result, rows.Err()
}
