package fundamentals

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
)

type fakeSymbolRepo struct {
	tracked []domain.Symbol
	err     error
}

func (f *fakeSymbolRepo) Upsert(ctx context.Context, symbols []domain.Symbol) error { return nil }
func (f *fakeSymbolRepo) Tracked(ctx context.Context) ([]domain.Symbol, error) {
	return f.tracked, f.err
}
func (f *fakeSymbolRepo) TrackedWithVolume(ctx context.Context) ([]domain.Symbol, error) {
	return f.tracked, f.err
}
func (f *fakeSymbolRepo) GetBySymbol(ctx context.Context, symbol string) (domain.Symbol, error) {
	return domain.Symbol{}, nil
}
func (f *fakeSymbolRepo) GetBatch(ctx context.Context, symbols []string) (map[string]domain.Symbol, error) {
	return nil, nil
}
func (f *fakeSymbolRepo) Search(ctx context.Context, query string, markets []string, page, size int) ([]domain.Symbol, int64, error) {
	return nil, 0, nil
}
func (f *fakeSymbolRepo) Deactivate(ctx context.Context, symbols []string) error { return nil }
func (f *fakeSymbolRepo) Markets(ctx context.Context) ([]string, error)          { return nil, nil }

type fakeFundamentalsRepo struct {
	batch map[string]domain.Fundamentals
	err   error
}

func (f *fakeFundamentalsRepo) Get(ctx context.Context, symbol string) (domain.Fundamentals, error) {
	return domain.Fundamentals{}, nil
}
func (f *fakeFundamentalsRepo) GetBatch(ctx context.Context, symbols []string) (map[string]domain.Fundamentals, error) {
	return f.batch, f.err
}
func (f *fakeFundamentalsRepo) UpsertDividends(ctx context.Context, fundamentals []domain.Fundamentals) error {
	return nil
}
func (f *fakeFundamentalsRepo) UpsertMarketMetrics(ctx context.Context, fundamentals []domain.Fundamentals) error {
	return nil
}
func (f *fakeFundamentalsRepo) UpsertExternalFundamentals(ctx context.Context, fundamentals []domain.Fundamentals) error {
	return nil
}
func (f *fakeFundamentalsRepo) GetSymbolsDueForFloatRefresh(ctx context.Context, limit int) ([]domain.Fundamentals, error) {
	return nil, nil
}
func (f *fakeFundamentalsRepo) UpsertEarningsHistory(ctx context.Context, fundamentals []domain.Fundamentals) error {
	return nil
}
func (f *fakeFundamentalsRepo) GetSymbolsWithStaleEarnings(ctx context.Context) ([]string, error) {
	return nil, nil
}
func (f *fakeFundamentalsRepo) UpsertBeta(ctx context.Context, fundamentals []domain.Fundamentals) error {
	return nil
}
func (f *fakeFundamentalsRepo) RecordStepDone(ctx context.Context, step string, at time.Time) error {
	return nil
}
func (f *fakeFundamentalsRepo) StepDoneAt(ctx context.Context, step string) (time.Time, bool, error) {
	return time.Time{}, false, nil
}
func (f *fakeFundamentalsRepo) GetSymbolsWithStaleBeta(ctx context.Context, windowStart time.Time) ([]string, error) {
	return nil, nil
}
func (f *fakeFundamentalsRepo) GetSymbolsWithStalePrevClose(ctx context.Context, windowStart time.Time) ([]string, error) {
	return nil, nil
}
func (f *fakeFundamentalsRepo) UpsertPrevClose(ctx context.Context, symbol string, close float64) error {
	return nil
}
func (f *fakeFundamentalsRepo) MarkPrevCloseAttempted(ctx context.Context, symbol string) error {
	return nil
}

func TestFundamentalsCache_ReloadAll_PopulatesFromRepo(t *testing.T) {
	symbols := &fakeSymbolRepo{tracked: []domain.Symbol{{Symbol: "AAPL"}, {Symbol: "MSFT"}}}
	repo := &fakeFundamentalsRepo{batch: map[string]domain.Fundamentals{
		"AAPL": {Symbol: "AAPL", MarketCap: 100},
		"MSFT": {Symbol: "MSFT", MarketCap: 200},
	}}
	cache := NewFundamentalsCache(repo, symbols, nil)

	cache.ReloadAll(context.Background())

	got := cache.GetBatch([]string{"AAPL", "MSFT", "GOOG"})
	if len(got) != 2 {
		t.Fatalf("expected 2 symbols, got %d: %+v", len(got), got)
	}
	if got["AAPL"].MarketCap != 100 || got["MSFT"].MarketCap != 200 {
		t.Fatalf("unexpected cached values: %+v", got)
	}
	if _, ok := got["GOOG"]; ok {
		t.Fatal("expected GOOG to be absent, not zero-valued")
	}
}

func TestFundamentalsCache_ReloadAll_KeepsPreviousDataOnError(t *testing.T) {
	symbols := &fakeSymbolRepo{tracked: []domain.Symbol{{Symbol: "AAPL"}}}
	repo := &fakeFundamentalsRepo{batch: map[string]domain.Fundamentals{"AAPL": {Symbol: "AAPL", MarketCap: 100}}}
	cache := NewFundamentalsCache(repo, symbols, nil)
	cache.ReloadAll(context.Background())

	repo.err = errors.New("db down")
	cache.ReloadAll(context.Background())

	got := cache.GetBatch([]string{"AAPL"})
	if got["AAPL"].MarketCap != 100 {
		t.Fatalf("expected stale-but-present data to survive a failed reload, got %+v", got)
	}
}

func TestFundamentalsCache_GetBatch_EmptyBeforeFirstReload(t *testing.T) {
	cache := NewFundamentalsCache(&fakeFundamentalsRepo{}, &fakeSymbolRepo{}, nil)
	got := cache.GetBatch([]string{"AAPL"})
	if len(got) != 0 {
		t.Fatalf("expected empty cache before any reload, got %+v", got)
	}
}

func TestFundamentalsCache_ReloadAll_PublishesToSubscribers(t *testing.T) {
	symbols := &fakeSymbolRepo{tracked: []domain.Symbol{{Symbol: "AAPL"}}}
	repo := &fakeFundamentalsRepo{batch: map[string]domain.Fundamentals{"AAPL": {Symbol: "AAPL", MarketCap: 100}}}
	broadcaster := livecandles.NewBroadcaster[domain.Fundamentals]()
	ch, cancel := broadcaster.Subscribe("AAPL")
	defer cancel()
	cache := NewFundamentalsCache(repo, symbols, broadcaster)

	cache.ReloadAll(context.Background())

	select {
	case got := <-ch:
		if got.MarketCap != 100 {
			t.Fatalf("published fundamentals = %+v, want MarketCap 100", got)
		}
	default:
		t.Fatal("expected ReloadAll to publish the reloaded fundamentals to subscribers")
	}
}
