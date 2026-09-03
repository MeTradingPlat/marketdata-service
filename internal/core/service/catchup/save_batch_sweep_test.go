package catchup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

type fakeSweepCandleRepo struct {
	saveCalls int
	lastBatch []domain.Candle
	saveErr   error
}

func (f *fakeSweepCandleRepo) Save(ctx context.Context, candles []domain.Candle, withWatermark bool) error {
	f.saveCalls++
	f.lastBatch = candles
	return f.saveErr
}
func (f *fakeSweepCandleRepo) GetCandles(context.Context, string, domain.Timeframe, int, *time.Time) ([]domain.Candle, error) {
	return nil, nil
}
func (f *fakeSweepCandleRepo) GetWatermark(context.Context, string, domain.Timeframe) (*time.Time, error) {
	return nil, nil
}
func (f *fakeSweepCandleRepo) GetWatermarksBatch(context.Context, []string, domain.Timeframe) (map[string]time.Time, error) {
	return nil, nil
}
func (f *fakeSweepCandleRepo) SymbolsWithData(context.Context, domain.Timeframe) (map[string]struct{}, error) {
	return nil, nil
}
func (f *fakeSweepCandleRepo) GetIntradaySessions(context.Context, string) (domain.IntradaySnapshot, error) {
	return domain.IntradaySnapshot{}, nil
}
func (f *fakeSweepCandleRepo) GetIntradaySessionsBatch(context.Context, []string) (map[string]domain.IntradaySnapshot, error) {
	return nil, nil
}
func (f *fakeSweepCandleRepo) GetPreviousSessionClose(context.Context, string, time.Time) (*float64, error) {
	return nil, nil
}
func (f *fakeSweepCandleRepo) GetPreviousSessionCloseBatch(context.Context, []string, time.Time) (map[string]float64, error) {
	return nil, nil
}
func (f *fakeSweepCandleRepo) GetSeriesPriority(context.Context, []string, domain.Timeframe, int) (map[string][]domain.Candle, error) {
	return nil, nil
}
func (f *fakeSweepCandleRepo) GetSeries(context.Context, []string, domain.Timeframe, int) (map[string][]domain.Candle, error) {
	return nil, nil
}
func (f *fakeSweepCandleRepo) GetSeriesAggregatedBatch(context.Context, []string, domain.Timeframe, domain.Timeframe, string, time.Duration, int) (map[string][]domain.Candle, error) {
	return nil, nil
}

type fakeSweepIngest struct {
	backfillCalls []string
}

func (f *fakeSweepIngest) Backfill(ctx context.Context, symbol string, tf domain.Timeframe) error {
	f.backfillCalls = append(f.backfillCalls, symbol)
	return nil
}
func (f *fakeSweepIngest) StreamLive(context.Context, string) error { return nil }
func (f *fakeSweepIngest) IsLive(string) bool                       { return false }
func (f *fakeSweepIngest) IsAttempted(string) bool                  { return false }
func (f *fakeSweepIngest) FlushLiveSaves(context.Context) bool      { return true }
func (f *fakeSweepIngest) RetryPendingSaves(context.Context) bool   { return true }

func TestSaveBatchSweep_MergesAllSymbolsIntoOneSaveCall(t *testing.T) {
	repo := &fakeSweepCandleRepo{}
	ingest := &fakeSweepIngest{}
	result := map[string][]domain.Candle{
		"AAPL": {{Symbol: "AAPL", Timeframe: domain.D1, Timestamp: time.Now().Add(-48 * time.Hour), Open: 1, High: 1, Low: 1, Close: 1}},
		"MSFT": {{Symbol: "MSFT", Timeframe: domain.D1, Timestamp: time.Now().Add(-48 * time.Hour), Open: 2, High: 2, Low: 2, Close: 2}},
	}

	saveBatchSweep(context.Background(), repo, ingest, result, domain.D1)

	if repo.saveCalls != 1 {
		t.Fatalf("Save() called %d times, want exactly 1 for the whole batch", repo.saveCalls)
	}
	if len(repo.lastBatch) != 2 {
		t.Fatalf("Save() got %d candles, want 2 (both symbols merged)", len(repo.lastBatch))
	}
}

func TestSaveBatchSweep_FallsBackPerSymbolOnSaveError(t *testing.T) {
	repo := &fakeSweepCandleRepo{saveErr: errors.New("db down")}
	ingest := &fakeSweepIngest{}
	result := map[string][]domain.Candle{
		"AAPL": {{Symbol: "AAPL", Timeframe: domain.D1, Timestamp: time.Now().Add(-48 * time.Hour), Close: 1}},
	}

	saveBatchSweep(context.Background(), repo, ingest, result, domain.D1)

	if len(ingest.backfillCalls) != 1 || ingest.backfillCalls[0] != "AAPL" {
		t.Fatalf("expected fallback Backfill(AAPL), got %v", ingest.backfillCalls)
	}
}

func TestSaveBatchSweep_NoClosedCandlesSkipsSave(t *testing.T) {
	repo := &fakeSweepCandleRepo{}
	ingest := &fakeSweepIngest{}
	result := map[string][]domain.Candle{
		"AAPL": {{Symbol: "AAPL", Timeframe: domain.M1, Timestamp: time.Now().Add(time.Hour)}},
	}

	saveBatchSweep(context.Background(), repo, ingest, result, domain.M1)

	if repo.saveCalls != 0 {
		t.Fatalf("Save() called %d times, want 0 when nothing closed", repo.saveCalls)
	}
}
