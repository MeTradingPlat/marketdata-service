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
func (f *fakeFundamentalsRepo) UpsertPrevCloseBatch(ctx context.Context, closes map[string]float64, attemptedOnly []string) error {
	return nil
}
func (f *fakeFundamentalsRepo) GetSymbolsWithStalePrevPostMarketVolume(ctx context.Context, windowStart time.Time) ([]string, error) {
	return nil, nil
}
func (f *fakeFundamentalsRepo) UpsertPrevPostMarketVolumeBatch(ctx context.Context, volumes map[string]int64, attemptedOnly []string) error {
	return nil
}
func (f *fakeFundamentalsRepo) GetSymbolsWithStaleOpenInterest(ctx context.Context, windowStart time.Time) ([]string, error) {
	return nil, nil
}
func (f *fakeFundamentalsRepo) UpsertOpenInterestBatch(ctx context.Context, openInterests map[string]float64, attemptedOnly []string) error {
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

func TestFundamentalsCache_ReloadAll_PreservesOldDataOnError(t *testing.T) {
	symbols := &fakeSymbolRepo{tracked: []domain.Symbol{{Symbol: "AAPL"}}}
	repo := &fakeFundamentalsRepo{batch: map[string]domain.Fundamentals{"AAPL": {Symbol: "AAPL", MarketCap: 100}}}
	cache := NewFundamentalsCache(repo, symbols, nil)
	cache.ReloadAll(context.Background())

	repo.err = errors.New("db down")
	cache.ReloadAll(context.Background())

	got := cache.GetBatch([]string{"AAPL"})
	if got["AAPL"].MarketCap != 100 {
		t.Fatalf("expected old data to survive db error, got %+v", got)
	}
}

func TestFundamentalsCache_MergeDividends_UpdatesOnlyDividendFields(t *testing.T) {
	cache := NewFundamentalsCache(&fakeFundamentalsRepo{}, &fakeSymbolRepo{}, nil)
	cache.MergeMarketMetrics([]domain.Fundamentals{{Symbol: "AAPL", MarketCap: 100, Beta: 1.2}})

	cache.MergeDividends([]domain.Fundamentals{{Symbol: "AAPL", DividendAmount: 0.25, TradingStatus: "T"}})

	got := cache.GetBatch([]string{"AAPL"})["AAPL"]
	if got.MarketCap != 100 || got.Beta != 1.2 {
		t.Errorf("expected existing metrics to be preserved, got %+v", got)
	}
	if got.DividendAmount != 0.25 || got.TradingStatus != "T" {
		t.Errorf("expected dividend fields to apply, got %+v", got)
	}
	if got.MarketDataUpdatedAt == nil {
		t.Error("expected MarketDataUpdatedAt to be stamped")
	}
}

func TestFundamentalsCache_MergeMarketMetrics_UpdatesOnlyMetricsFields(t *testing.T) {
	cache := NewFundamentalsCache(&fakeFundamentalsRepo{}, &fakeSymbolRepo{}, nil)
	cache.MergeDividends([]domain.Fundamentals{{Symbol: "AAPL", DividendAmount: 0.25}})

	cache.MergeMarketMetrics([]domain.Fundamentals{{Symbol: "AAPL", MarketCap: 100, Eps: 3.5}})

	got := cache.GetBatch([]string{"AAPL"})["AAPL"]
	if got.DividendAmount != 0.25 {
		t.Errorf("expected existing dividends to be preserved, got %+v", got)
	}
	if got.MarketCap != 100 || got.Eps != 3.5 {
		t.Errorf("expected metrics to apply, got %+v", got)
	}
	if got.MetricsUpdatedAt == nil {
		t.Error("expected MetricsUpdatedAt to be stamped")
	}
}

func TestFundamentalsCache_MergeExternalFundamentals_CoalescesPointers(t *testing.T) {
	cache := NewFundamentalsCache(&fakeFundamentalsRepo{}, &fakeSymbolRepo{}, nil)
	shares := int64(1000)
	cache.MergeExternalFundamentals([]domain.Fundamentals{{Symbol: "AAPL", SharesOutstanding: &shares}})

	floatShares := int64(800)
	cache.MergeExternalFundamentals([]domain.Fundamentals{{Symbol: "AAPL", FloatShares: &floatShares}})

	got := cache.GetBatch([]string{"AAPL"})["AAPL"]
	if got.SharesOutstanding == nil || *got.SharesOutstanding != 1000 {
		t.Errorf("expected sharesOutstanding to survive a later update that left it nil, got %+v", got.SharesOutstanding)
	}
	if got.FloatShares == nil || *got.FloatShares != 800 {
		t.Errorf("expected floatShares to apply, got %+v", got)
	}
}

// TestFundamentalsCache_MergeEarningsHistory_EmptyDateDoesNotOverwrite
// reproduce el NULLIF+COALESCE de UpsertEarningsHistory: una prediccion
// vacia (sin earnings futuro que predecir esta vuelta) no debe borrar la
// fecha vigente que ya estaba.
func TestFundamentalsCache_MergeEarningsHistory_EmptyDateDoesNotOverwrite(t *testing.T) {
	cache := NewFundamentalsCache(&fakeFundamentalsRepo{}, &fakeSymbolRepo{}, nil)
	cache.MergeEarningsHistory([]domain.Fundamentals{{Symbol: "AAPL", NextEarningsDate: "2026-10-30"}})

	cache.MergeEarningsHistory([]domain.Fundamentals{{Symbol: "AAPL", NextEarningsDate: ""}})

	got := cache.GetBatch([]string{"AAPL"})["AAPL"]
	if got.NextEarningsDate != "2026-10-30" {
		t.Errorf("expected the existing earnings date to survive an empty update, got %q", got.NextEarningsDate)
	}
}

func TestFundamentalsCache_MergeOpenInterest(t *testing.T) {
	cache := NewFundamentalsCache(&fakeFundamentalsRepo{}, &fakeSymbolRepo{}, nil)
	cache.MergeOpenInterest(map[string]float64{"AAPL": 12500}, []string{"FTFT"})

	got := cache.GetBatch([]string{"AAPL", "FTFT"})
	aapl := got["AAPL"]
	if aapl.OpenInterest == nil || *aapl.OpenInterest != 12500 {
		t.Errorf("expected AAPL open interest = 12500, got %+v", aapl.OpenInterest)
	}
	if aapl.OpenInterestUpdatedAt == nil {
		t.Error("expected AAPL open interest timestamp to be set")
	}

	ftft := got["FTFT"]
	if ftft.OpenInterest != nil {
		t.Errorf("expected FTFT open interest to be nil, got %+v", ftft.OpenInterest)
	}
	if ftft.OpenInterestUpdatedAt == nil {
		t.Error("expected FTFT open interest timestamp to be set")
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
