package catchup

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
)

type fakePartialGateway struct {
	out.MarketDataGateway
}

func (f *fakePartialGateway) GetCandlesBatch(ctx context.Context, tf domain.Timeframe, froms map[string]time.Time) (map[string][]domain.Candle, error) {
	now := time.Now()
	return map[string][]domain.Candle{
		"AAPL": {{Symbol: "AAPL", Timeframe: domain.D1, Timestamp: now.Add(-48 * time.Hour), Open: 1, High: 1, Low: 1, Close: 1}},
	}, domain.ErrPartialHistory
}

func TestBackfillBatchWithRetry_PartialHistory(t *testing.T) {
	now := time.Now()
	gw := &fakePartialGateway{}
	repo := &fakeSweepCandleRepo{
		watermarks: map[string]time.Time{
			"AAPL": now.Add(-72 * time.Hour),
			"MSFT": now.Add(-72 * time.Hour),
		},
	}
	ingest := &fakeSweepIngest{}

	jobs := []job{{symbol: "AAPL", tf: domain.D1}, {symbol: "MSFT", tf: domain.D1}}
	got, partial := backfillBatchWithRetry(context.Background(), gw, repo, ingest, jobs, domain.D1, 24*time.Hour)
	if !partial {
		t.Errorf("partial = false, want true")
	}

	if repo.saveCalls != 1 {
		t.Errorf("repo.saveCalls = %d, want 1", repo.saveCalls)
	}
	if repo.lastWithWatermark != false {
		t.Errorf("repo.lastWithWatermark = %v, want false", repo.lastWithWatermark)
	}
	if !reflect.DeepEqual(got, []string{"MSFT"}) {
		t.Errorf("got %v, want [MSFT]", got)
	}
	if len(ingest.backfillCalls) != 0 {
		t.Errorf("len(ingest.backfillCalls) = %d, want 0", len(ingest.backfillCalls))
	}
}
