package timescale

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// GetSymbolsWithStaleOpenInterest trae los simbolos cuyo open_interest no se
// calculo en la ventana de mantenimiento actual (mismo guard por-simbolo).
func (r *FundamentalsRepository) GetSymbolsWithStaleOpenInterest(ctx context.Context, windowStart time.Time) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT s.symbol
		FROM tracked_symbols s
		LEFT JOIN dividends d ON d.symbol_id = s.symbol_id
		WHERE s.is_active = TRUE
		  AND s.symbol !~ '/'
		  AND (d.open_interest_updated_at IS NULL OR d.open_interest_updated_at < $1)
		ORDER BY s.symbol`, windowStart)
	if err != nil {
		return nil, fmt.Errorf("listing symbols with stale open interest: %w", err)
	}
	defer rows.Close()
	var symbols []string
	for rows.Next() {
		var symbol string
		if err := rows.Scan(&symbol); err != nil {
			return nil, fmt.Errorf("scanning stale open interest symbol: %w", err)
		}
		symbols = append(symbols, symbol)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating stale open interest symbols: %w", err)
	}
	return symbols, nil
}

const upsertOpenInterestSQL = `
	INSERT INTO dividends (symbol_id, open_interest, open_interest_updated_at)
	SELECT symbol_id, $2, now() FROM tracked_symbols WHERE symbol = $1
	ON CONFLICT (symbol_id) DO UPDATE SET open_interest = EXCLUDED.open_interest, open_interest_updated_at = now()
`

const markOpenInterestAttemptedSQL = `
	INSERT INTO dividends (symbol_id, open_interest_updated_at)
	SELECT symbol_id, now() FROM tracked_symbols WHERE symbol = $1
	ON CONFLICT (symbol_id) DO UPDATE SET open_interest_updated_at = now()
`

// UpsertOpenInterestBatch guarda el open interest calculado para el lote de
// simbolos que tuvieron opciones y estampa open_interest_updated_at para los
// que se marcaron como intentados sin opciones.
func (r *FundamentalsRepository) UpsertOpenInterestBatch(ctx context.Context, openInterests map[string]float64, attemptedOnly []string) error {
	order := lockOrder(openInterests, attemptedOnly)
	if len(order) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, symbol := range order {
		if oi, ok := openInterests[symbol]; ok {
			batch.Queue(upsertOpenInterestSQL, symbol, oi)
		} else {
			batch.Queue(markOpenInterestAttemptedSQL, symbol)
		}
	}
	results := r.pool.SendBatch(ctx, batch)
	defer results.Close()
	for range order {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("upserting open interest batch: %w", err)
		}
	}
	return nil
}
