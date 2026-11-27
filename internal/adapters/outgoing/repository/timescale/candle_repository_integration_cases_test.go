package timescale

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

type upsertMatrixCase struct {
	existingVerified bool
	newVerified      bool
	setNull          bool
	delta            time.Duration
	wantUpdated      bool
}

func buildUpsertMatrix() []upsertMatrixCase {
	var cases []upsertMatrixCase
	for _, ev := range []bool{false, true} {
		for _, nv := range []bool{false, true} {
			for _, d := range []time.Duration{-time.Minute, 0, time.Minute} {
				want := (nv && !ev) || (nv == ev && d >= 0)
				cases = append(cases, upsertMatrixCase{ev, nv, false, d, want})
			}
			cases = append(cases, upsertMatrixCase{ev, nv, true, -time.Minute, nv || !ev})
		}
	}
	return cases
}

func compressIntegrationChunks(t *testing.T, pool *pgxpool.Pool, table string, from, to time.Time) {
	t.Helper()
	q := fmt.Sprintf(`SELECT compress_chunk(c, if_not_compressed => true) FROM show_chunks('%s', older_than => $1::timestamptz, newer_than => $2::timestamptz) c`, table)
	if _, err := pool.Exec(context.Background(), q, to, from); err != nil {
		t.Fatalf("compress_chunk %s: %v", table, err)
	}
}

func seedMatrixCandles(t *testing.T, repo *CandleRepository, pool *pgxpool.Pool, table, sym string, tf domain.Timeframe, baseTS, obs time.Time, cases []upsertMatrixCase) {
	t.Helper()
	ctx := context.Background()
	for i, tc := range cases {
		ts := baseTS.Add(time.Duration(i) * time.Minute)
		c := domain.Candle{Symbol: sym, Timeframe: tf, Timestamp: ts, Open: 10, High: 10, Low: 10, Close: 10, ObservedAt: obs}
		if err := repo.Save(ctx, []domain.Candle{c}, tc.existingVerified); err != nil {
			t.Fatalf("seed case %d: %v", i, err)
		}
		if tc.setNull {
			q := fmt.Sprintf(`UPDATE %s SET observed_at = NULL WHERE symbol_id = (SELECT symbol_id FROM tracked_symbols WHERE symbol = $1) AND timeframe = $2 AND ts = $3`, table)
			if _, err := pool.Exec(ctx, q, sym, string(tf), ts); err != nil {
				t.Fatalf("null observed_at case %d: %v", i, err)
			}
		}
	}
}
