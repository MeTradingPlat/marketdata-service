package catchup

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
)

type fakeBatchGateway struct {
	out.MarketDataGateway
	live   map[string]bool
	result map[string][]domain.Candle
	err    error
}

func (f *fakeBatchGateway) LiveSubscribed(sym string) bool { return f.live[sym] }

func (f *fakeBatchGateway) GetCandlesBatch(context.Context, domain.Timeframe, map[string]time.Time) (map[string][]domain.Candle, error) {
	return f.result, f.err
}

func TestBatchHelpers_AndBackfillBatchWithRetry(t *testing.T) {
	now := time.Now()
	gw := &fakeBatchGateway{
		live: map[string]bool{"NVDA": true},
		result: map[string][]domain.Candle{
			"AAPL": {{Symbol: "AAPL", Timeframe: domain.D1, Timestamp: now.Add(-48 * time.Hour), Open: 1, High: 1, Low: 1, Close: 1}},
		},
	}
	repo := &fakeSweepCandleRepo{watermarks: map[string]time.Time{"AAPL": now.Add(-72 * time.Hour)}}
	ingest := &fakeSweepIngest{}
	jobs := []job{{symbol: "NVDA", tf: domain.M1}, {symbol: "AAPL", tf: domain.M1}}

	syms, skipped := filterBatchNonLive(gw, jobs, domain.M1)
	if !reflect.DeepEqual(syms, []string{"AAPL"}) || skipped != 1 {
		t.Fatalf("filterBatchNonLive = (%v, %d), want ([AAPL], 1)", syms, skipped)
	}
	froms := buildBatchFroms([]string{"AAPL", "NEW"}, repo.watermarks, domain.M1, time.Minute)
	if len(froms) != 2 {
		t.Fatalf("len(buildBatchFroms) = %d, want 2", len(froms))
	}
	if empty := emptyBatchSymbols(froms, gw.result); !reflect.DeepEqual(empty, []string{"NEW"}) {
		t.Fatalf("emptyBatchSymbols = %v, want [NEW]", empty)
	}

	empty, partial, gotSkipped := backfillBatchWithRetry(context.Background(), gw, repo, ingest, jobs, domain.M1, time.Minute)
	if partial || gotSkipped != 1 || len(empty) != 0 || repo.saveCalls != 1 {
		t.Fatalf("backfillBatchWithRetry = (%v, %v, %d, saves=%d)", empty, partial, gotSkipped, repo.saveCalls)
	}

	gw.err = errors.New("boom")
	backfillBatchWithRetry(context.Background(), gw, repo, ingest, []job{{symbol: "AAPL", tf: domain.D1}}, domain.D1, 24*time.Hour)
	if len(ingest.backfillCalls) == 0 {
		t.Fatal("expected fallback to per-symbol backfill on error")
	}
}
