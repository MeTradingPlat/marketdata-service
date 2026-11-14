package timescale

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// RecordStepDone registra cuando termino un refresh diario de fundamentales
// (beta, earnings, market metrics, externos) en Postgres -- en reinicios el
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

// GetSymbolsWithStaleBeta trae los simbolos cuyo beta NO se calculo en la
// ventana de mantenimiento actual (nunca calculado, o con fecha anterior al
// arranque de ventana) -- el refresh de beta solo calcula estos, no el
// universo entero: un redeploy a mitad de dia recalcula solo los vencidos
// (ver refreshFundamentalsOnce y el guard por-simbolo beta_updated_at).
func (r *FundamentalsRepository) GetSymbolsWithStaleBeta(ctx context.Context, windowStart time.Time) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT s.symbol
		FROM tracked_symbols s
		LEFT JOIN dividends d ON d.symbol_id = s.symbol_id
		WHERE s.is_active = TRUE
		  AND (d.beta_updated_at IS NULL OR d.beta_updated_at < $1)
		ORDER BY s.symbol`, windowStart)
	if err != nil {
		return nil, fmt.Errorf("listing symbols with stale beta: %w", err)
	}
	defer rows.Close()
	var symbols []string
	for rows.Next() {
		var symbol string
		if err := rows.Scan(&symbol); err != nil {
			return nil, fmt.Errorf("scanning stale beta symbol: %w", err)
		}
		symbols = append(symbols, symbol)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating stale beta symbols: %w", err)
	}
	return symbols, nil
}

// GetSymbolsWithStalePrevClose es el mismo guard por-simbolo para el
// prevClose calculado en el backfill (columna prev_close_updated_at).
func (r *FundamentalsRepository) GetSymbolsWithStalePrevClose(ctx context.Context, windowStart time.Time) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT s.symbol
		FROM tracked_symbols s
		LEFT JOIN dividends d ON d.symbol_id = s.symbol_id
		WHERE s.is_active = TRUE
		  AND (d.prev_close_updated_at IS NULL OR d.prev_close_updated_at < $1)
		ORDER BY s.symbol`, windowStart)
	if err != nil {
		return nil, fmt.Errorf("listing symbols with stale prev close: %w", err)
	}
	defer rows.Close()
	var symbols []string
	for rows.Next() {
		var symbol string
		if err := rows.Scan(&symbol); err != nil {
			return nil, fmt.Errorf("scanning stale prev close symbol: %w", err)
		}
		symbols = append(symbols, symbol)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating stale prev close symbols: %w", err)
	}
	return symbols, nil
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
