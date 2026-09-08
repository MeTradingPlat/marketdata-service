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

// TestFundamentalsCache_MergeMarketMetrics_DoesNotWipeOtherRefreshesFields
// reproduce el motivo real de tener Merge* separados en vez de un solo
// reemplazo del registro: dividendos y market metrics conviven en el mismo
// Fundamentals pero los escribe cada uno en un momento distinto del ciclo
// -- pisar el registro entero con lo que trae SOLO market metrics borraria
// el dividendo que MergeDividends ya habia guardado antes. (Beta es
// intencionalmente la excepcion: tanto market metrics como el refresh de
// beta propio escriben esa MISMA columna sin COALESCE -- ver
// upsertMarketMetricsSQL/upsertBetaSQL -- el que corre despues gana, por
// eso RefreshBeta corre DESPUES de RefreshMarketMetrics en el ciclo real.)
func TestFundamentalsCache_MergeMarketMetrics_DoesNotWipeOtherRefreshesFields(t *testing.T) {
	cache := NewFundamentalsCache(&fakeFundamentalsRepo{}, &fakeSymbolRepo{}, nil)
	cache.MergeDividends([]domain.Fundamentals{{Symbol: "AAPL", DividendAmount: 0.25}})

	cache.MergeMarketMetrics([]domain.Fundamentals{{Symbol: "AAPL", MarketCap: 3_000_000_000}})

	got := cache.GetBatch([]string{"AAPL"})["AAPL"]
	if got.DividendAmount != 0.25 {
		t.Errorf("expected dividend from the earlier MergeDividends to survive, got %v", got.DividendAmount)
	}
	if got.MarketCap != 3_000_000_000 {
		t.Errorf("expected the new market cap to apply, got %v", got.MarketCap)
	}
}

// TestFundamentalsCache_MergeExternalFundamentals_KeepsExistingWhenNil
// reproduce el COALESCE de UpsertExternalFundamentals en memoria: un update
// sin sharesOutstanding (nil, ej. DxLink no lo trajo esta vuelta) no debe
// pisar el que SEC EDGAR ya habia completado antes.
func TestFundamentalsCache_MergeExternalFundamentals_KeepsExistingWhenNil(t *testing.T) {
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
		t.Errorf("expected floatShares to apply, got %+v", got.FloatShares)
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
