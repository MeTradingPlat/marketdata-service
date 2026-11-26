package catchup

import (
	"context"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestRunPhase_CoversUncoveredAndCovered(t *testing.T) {
	now := time.Now()
	gw := &fakeBatchGateway{
		live: map[string]bool{"NVDA": true},
		result: map[string][]domain.Candle{
			"AAPL": {{Symbol: "AAPL", Timeframe: domain.M1, Timestamp: now.Add(-2 * time.Minute), Open: 1, High: 1, Low: 1, Close: 1}},
		},
	}
	repo := &fakeSweepCandleRepo{watermarks: map[string]time.Time{"AAPL": now.Add(-10 * time.Minute)}}
	ingest := &fakeSweepIngest{}

	uncovered := []job{{symbol: "UNC", tf: domain.M1}}
	covered := []job{{symbol: "AAPL", tf: domain.M1}, {symbol: "NVDA", tf: domain.M1}}
	runPhase(context.Background(), gw, repo, ingest, uncovered, covered, domain.M1, 2)

	if len(ingest.backfillCalls) != 1 || ingest.backfillCalls[0] != "UNC" {
		t.Fatalf("backfillCalls = %v, want [UNC]", ingest.backfillCalls)
	}
	if repo.saveCalls != 1 {
		t.Fatalf("repo.saveCalls = %d, want 1", repo.saveCalls)
	}
}

func TestRunPhase_InvalidTimeframeAndMergeBatchResult(t *testing.T) {
	runPhase(context.Background(), &fakeBatchGateway{}, &fakeSweepCandleRepo{}, &fakeSweepIngest{}, nil, nil, domain.Timeframe("BAD"), 1)

	s := newPhaseSummary([]job{{symbol: "A"}}, nil)
	s.merge(batchOutcome{empty: []string{"A"}, partial: true, skippedLive: 2})
	if len(s.empty) != 1 || s.partialBatches != 1 || s.skippedLive != 2 {
		t.Fatalf("merge = (%v, %d, %d), want ([A], 1, 2)", s.empty, s.partialBatches, s.skippedLive)
	}
	s.merge(batchOutcome{})
	if len(s.empty) != 1 || s.partialBatches != 1 || s.skippedLive != 2 {
		t.Fatalf("no-op merge changed state: (%v, %d, %d)", s.empty, s.partialBatches, s.skippedLive)
	}
	logPhaseSummary(domain.D1, s, 1, []string{"A"}, time.Second)
}
