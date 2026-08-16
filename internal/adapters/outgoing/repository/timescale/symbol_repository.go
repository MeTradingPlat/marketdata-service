package timescale

import (
	"context"
	"errors"
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
	INSERT INTO tracked_symbols (symbol, market, is_etf, description) VALUES ($1, $2, $3, $4)
	ON CONFLICT (symbol) DO UPDATE SET
		market = EXCLUDED.market, is_active = TRUE, is_etf = EXCLUDED.is_etf, description = EXCLUDED.description
`

func (r *SymbolRepository) Upsert(ctx context.Context, symbols []domain.Symbol) error {
	if len(symbols) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, s := range symbols {
		batch.Queue(upsertSymbolSQL, s.Symbol, s.Market, s.IsEtf, s.Description)
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

const trackedSymbolsSQL = `SELECT symbol, market, description, is_etf FROM tracked_symbols WHERE is_active = TRUE`

func (r *SymbolRepository) Tracked(ctx context.Context) ([]domain.Symbol, error) {
	rows, err := r.pool.Query(ctx, trackedSymbolsSQL)
	if err != nil {
		return nil, fmt.Errorf("querying tracked symbols: %w", err)
	}
	defer rows.Close()

	var symbols []domain.Symbol
	for rows.Next() {
		var s domain.Symbol
		if err := rows.Scan(&s.Symbol, &s.Market, &s.Description, &s.IsEtf); err != nil {
			return nil, fmt.Errorf("scanning symbol row: %w", err)
		}
		symbols = append(symbols, s)
	}
	return symbols, rows.Err()
}

const marketsSQL = `SELECT DISTINCT market FROM tracked_symbols WHERE is_active = TRUE ORDER BY market`

func (r *SymbolRepository) Markets(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, marketsSQL)
	if err != nil {
		return nil, fmt.Errorf("querying distinct markets: %w", err)
	}
	defer rows.Close()

	var markets []string
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return nil, fmt.Errorf("scanning market row: %w", err)
		}
		markets = append(markets, m)
	}
	return markets, rows.Err()
}

const getSymbolSQL = `SELECT symbol, market, description, is_etf FROM tracked_symbols WHERE symbol = $1 AND is_active = TRUE`

func (r *SymbolRepository) GetBySymbol(ctx context.Context, symbol string) (domain.Symbol, error) {
	var s domain.Symbol
	err := r.pool.QueryRow(ctx, getSymbolSQL, symbol).Scan(&s.Symbol, &s.Market, &s.Description, &s.IsEtf)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Symbol{}, fmt.Errorf("symbol %s not tracked", symbol)
	}
	if err != nil {
		return domain.Symbol{}, fmt.Errorf("querying symbol %s: %w", symbol, err)
	}
	return s, nil
}

// searchSymbolsSQL ordena por last_volume (el D1 mas reciente de cada
// simbolo, ver Save()) en vez de alfabetico -- mucho mas util para un
// screener: los simbolos con mas actividad real aparecen primero. El
// COUNT(*) OVER() trae el total en la misma consulta, sin un segundo
// round-trip solo para paginar. La tabla es chica (un symbol_id por fila,
// no el hypertable de candles), un ILIKE completo no necesita mas.
const searchSymbolsSQL = `
	SELECT symbol, market, description, is_etf, COUNT(*) OVER() AS total
	FROM tracked_symbols
	WHERE is_active = TRUE
		AND ($1 = '' OR symbol ILIKE '%' || $1 || '%' OR description ILIKE '%' || $1 || '%')
		AND ($2::text[] IS NULL OR market = ANY($2))
	ORDER BY last_volume DESC, symbol ASC
	LIMIT $3 OFFSET $4
`

func (r *SymbolRepository) Search(ctx context.Context, query string, markets []string, page, size int) ([]domain.Symbol, int64, error) {
	var marketsParam []string
	if len(markets) > 0 {
		marketsParam = markets
	}

	rows, err := r.pool.Query(ctx, searchSymbolsSQL, query, marketsParam, size, page*size)
	if err != nil {
		return nil, 0, fmt.Errorf("searching symbols: %w", err)
	}
	defer rows.Close()

	var symbols []domain.Symbol
	var total int64
	for rows.Next() {
		var s domain.Symbol
		if err := rows.Scan(&s.Symbol, &s.Market, &s.Description, &s.IsEtf, &total); err != nil {
			return nil, 0, fmt.Errorf("scanning symbol search row: %w", err)
		}
		symbols = append(symbols, s)
	}
	return symbols, total, rows.Err()
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
