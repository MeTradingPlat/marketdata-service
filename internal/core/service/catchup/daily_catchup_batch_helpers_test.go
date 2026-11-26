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

	syms, skipped := filterBatchNonLiveSymbols(gw, jobs, domain.M1)
	if !reflect.DeepEqual(syms, []string{"AAPL"}) || !reflect.DeepEqual(skipped, []string{"NVDA"}) {
		t.Fatalf("filterBatchNonLiveSymbols = (%v, %v), want ([AAPL], [NVDA])", syms, skipped)
	}
	froms, _, _ := partitionBatchWatermarks([]string{"AAPL", "NEW"}, repo.watermarks, domain.M1, time.Minute)
	if len(froms) != 2 {
		t.Fatalf("len(partitionBatchWatermarks) = %d, want 2", len(froms))
	}
	if empty := emptyBatchSymbols(froms, gw.result); !reflect.DeepEqual(empty, []string{"NEW"}) {
		t.Fatalf("emptyBatchSymbols = %v, want [NEW]", empty)
	}

	o := backfillBatchWithOutcome(context.Background(), gw, repo, ingest, jobs, domain.M1, time.Minute)
	if o.partial || o.skippedLive != 1 || len(o.empty) != 0 || repo.saveCalls != 1 {
		t.Fatalf("backfillBatchWithOutcome = (%+v, saves=%d)", o, repo.saveCalls)
	}

	gw.err = errors.New("boom")
	backfillBatchWithOutcome(context.Background(), gw, repo, ingest, []job{{symbol: "AAPL", tf: domain.D1}}, domain.D1, 24*time.Hour)
	if len(ingest.backfillCalls) == 0 {
		t.Fatal("expected fallback to per-symbol backfill on error")
	}
}
