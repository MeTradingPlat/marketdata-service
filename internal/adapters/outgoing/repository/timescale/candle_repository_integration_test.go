package timescale

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

func runUpsertMatrixSuite(t *testing.T, repo *CandleRepository, pool *pgxpool.Pool, table, sym string, tf domain.Timeframe, compress bool) {
	t.Helper()
	setupIntegrationSymbol(t, pool, sym)
	cases := buildUpsertMatrix()
	baseTS := time.Date(2020, 1, 2, 10, 0, 0, 0, time.UTC)
	obs := time.Date(2026, 11, 27, 15, 0, 0, 0, time.UTC)
	seedMatrixCandles(t, repo, pool, table, sym, tf, baseTS, obs, cases)
	if compress {
		compressIntegrationChunks(t, pool, table, baseTS.Add(-24*time.Hour), baseTS.Add(48*time.Hour))
	}
	for i, tc := range cases {
		ts := baseTS.Add(time.Duration(i) * time.Minute)
		c := domain.Candle{Symbol: sym, Timeframe: tf, Timestamp: ts, Open: 20, High: 20, Low: 20, Close: 20, ObservedAt: obs.Add(tc.delta)}
		if err := repo.Save(context.Background(), []domain.Candle{c}, tc.newVerified); err != nil {
			t.Fatalf("update case %d: %v", i, err)
		}
		wantClose := 10.0
		if tc.wantUpdated {
			wantClose = 20.0
		}
		if got := readStoredClose(t, pool, table, sym, tf, ts); got != wantClose {
			t.Fatalf("%s compress=%v case %d (%+v): close=%v, want %v", table, compress, i, tc, got, wantClose)
		}
	}
}

func TestCandleRepository_Integration_UpsertObservedAt(t *testing.T) {
	repo, pool := openIntegrationRepo(t)
	for _, compress := range []bool{false, true} {
		runUpsertMatrixSuite(t, repo, pool, "candles", fmt.Sprintf("ITEST_M1_%v", compress), domain.M1, compress)
		runUpsertMatrixSuite(t, repo, pool, "candles_history", fmt.Sprintf("ITEST_H1_%v", compress), domain.H1, compress)
	}
}

func TestCandleRepository_Integration_Save8000Compressed(t *testing.T) {
	repo, pool := openIntegrationRepo(t)
	sym := "ITEST_8000"
	setupIntegrationSymbol(t, pool, sym)
	baseTS := time.Date(2020, 2, 1, 0, 0, 0, 0, time.UTC)
	obs := time.Date(2026, 11, 27, 15, 0, 0, 0, time.UTC)
	batch := make([]domain.Candle, 8000)
	for i := range batch {
		batch[i] = domain.Candle{Symbol: sym, Timeframe: domain.M1, Timestamp: baseTS.Add(time.Duration(i) * time.Minute), Open: 1, High: 1, Low: 1, Close: 1, ObservedAt: obs}
	}
	if err := repo.Save(context.Background(), batch, true); err != nil {
		t.Fatalf("initial Save 8000: %v", err)
	}
	compressIntegrationChunks(t, pool, "candles", baseTS.Add(-24*time.Hour), baseTS.Add(10*24*time.Hour))
	for i := range batch {
		batch[i].Close = 2
		batch[i].ObservedAt = obs.Add(time.Minute)
	}
	if err := repo.Save(context.Background(), batch, true); err != nil {
		t.Fatalf("compressed Save 8000: %v", err)
	}
}
