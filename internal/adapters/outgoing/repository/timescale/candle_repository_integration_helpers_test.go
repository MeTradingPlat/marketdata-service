package timescale

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

func openIntegrationRepo(t *testing.T) (*CandleRepository, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("TEST_DB_URL")
	if dsn == "" {
		t.Skip("TEST_DB_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	return NewCandleRepository(pool, pool, pool), pool
}

func setupIntegrationSymbol(t *testing.T, pool *pgxpool.Pool, sym string) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO tracked_symbols (symbol, market) VALUES ($1, 'XNAS') ON CONFLICT (symbol) DO NOTHING`, sym); err != nil {
		t.Fatalf("insert tracked_symbol: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM candles WHERE symbol_id IN (SELECT symbol_id FROM tracked_symbols WHERE symbol = $1)`, sym)
		_, _ = pool.Exec(ctx, `DELETE FROM candles_history WHERE symbol_id IN (SELECT symbol_id FROM tracked_symbols WHERE symbol = $1)`, sym)
		_, _ = pool.Exec(ctx, `DELETE FROM watermarks WHERE symbol_id IN (SELECT symbol_id FROM tracked_symbols WHERE symbol = $1)`, sym)
		_, _ = pool.Exec(ctx, `DELETE FROM tracked_symbols WHERE symbol = $1`, sym)
	})
}

func readStoredClose(t *testing.T, pool *pgxpool.Pool, table, sym string, tf domain.Timeframe, ts time.Time) float64 {
	t.Helper()
	var c float64
	q := fmt.Sprintf(`SELECT c.close FROM %s c JOIN tracked_symbols s ON s.symbol_id = c.symbol_id WHERE s.symbol = $1 AND c.timeframe = $2 AND c.ts = $3`, table)
	if err := pool.QueryRow(context.Background(), q, sym, string(tf), ts).Scan(&c); err != nil {
		t.Fatalf("scan close: %v", err)
	}
	return c
}
