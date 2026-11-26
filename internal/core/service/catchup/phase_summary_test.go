package catchup

import (
	"context"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestPhaseSummary_MissingAndSample(t *testing.T) {
	uncovered := []job{{symbol: "U1"}}
	covered := []job{{symbol: "C1"}, {symbol: "C2"}, {symbol: "C3"}}
	s := newPhaseSummary(uncovered, covered)
	s.merge(batchOutcome{saved: 1, upToDate: 1, accounted: []string{"U1", "C1"}})

	missingCount, sample := s.missing(uncovered, covered)
	if missingCount != 2 || len(sample) != 2 || sample[0] != "C2" || sample[1] != "C3" {
		t.Fatalf("missing = (%d, %v), want (2, [C2 C3])", missingCount, sample)
	}
}

func TestBackfillBatchWithOutcome_D1CoveredWithoutWatermarkReprocesses(t *testing.T) {
	now := time.Now()
	gw := &fakeBatchGateway{result: map[string][]domain.Candle{}}
	repo := &fakeSweepCandleRepo{
		watermarks: map[string]time.Time{"UPTODATE": now.Add(-1 * time.Hour)},
	}
	ingest := &fakeSweepIngest{}
	batch := []job{
		{symbol: "NOWM", tf: domain.D1},
		{symbol: "UPTODATE", tf: domain.D1},
	}

	o := backfillBatchWithOutcome(context.Background(), gw, repo, ingest, batch, domain.D1, 24*time.Hour)
	if o.noWatermark != 1 || o.upToDate != 1 || o.saved != 1 {
		t.Fatalf("unexpected outcome: %+v", o)
	}
	if len(ingest.backfillCalls) != 1 || ingest.backfillCalls[0] != "NOWM" {
		t.Fatalf("backfillCalls = %v, want [NOWM]", ingest.backfillCalls)
	}
}
