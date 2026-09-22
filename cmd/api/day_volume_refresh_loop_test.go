package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/intraday"
)

type fakeTrackedSymbols struct {
	tracked []domain.Symbol
}

func (f *fakeTrackedSymbols) Upsert(context.Context, []domain.Symbol) error    { return nil }
func (f *fakeTrackedSymbols) Tracked(context.Context) ([]domain.Symbol, error) { return f.tracked, nil }
func (f *fakeTrackedSymbols) TrackedWithVolume(context.Context) ([]domain.Symbol, error) {
	return f.tracked, nil
}
func (f *fakeTrackedSymbols) GetBySymbol(context.Context, string) (domain.Symbol, error) {
	return domain.Symbol{}, nil
}
func (f *fakeTrackedSymbols) GetBatch(context.Context, []string) (map[string]domain.Symbol, error) {
	return nil, nil
}
func (f *fakeTrackedSymbols) Search(context.Context, string, []string, int, int) ([]domain.Symbol, int64, error) {
	return nil, 0, nil
}
func (f *fakeTrackedSymbols) Deactivate(context.Context, []string) error { return nil }
func (f *fakeTrackedSymbols) Markets(context.Context) ([]string, error)  { return nil, nil }

type fakeDayVolumeRepo struct {
	mu       sync.Mutex
	seed     map[string]int64
	saved    map[string]int64
	savedDay time.Time
}

func (f *fakeDayVolumeRepo) GetBatch(_ context.Context, _ []string, _ time.Time) (map[string]int64, error) {
	return f.seed, nil
}
func (f *fakeDayVolumeRepo) SaveBatch(_ context.Context, day time.Time, volumes map[string]int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saved = volumes
	f.savedDay = day
	return nil
}

func TestSeedDayVolumesFromDB_LoadsIntoTheTrackerBeforeAnyLiveRefresh(t *testing.T) {
	symbols := &fakeTrackedSymbols{tracked: []domain.Symbol{{Symbol: "SPY"}, {Symbol: "AAPL"}}}
	repo := &fakeDayVolumeRepo{seed: map[string]int64{"SPY": 5_468_232}}
	tracker := intraday.NewDayVolumeTracker()

	seedDayVolumesFromDB(context.Background(), repo, symbols, tracker)

	got, ok := tracker.Get("SPY")
	if !ok || got != 5_468_232 {
		t.Fatalf("SPY = %d ok=%v, want 5468232 true", got, ok)
	}
	if _, ok := tracker.Get("AAPL"); ok {
		t.Fatal("AAPL had no seed row, should not appear")
	}
}

func TestRefreshDayVolumes_UpdatesTheTrackerAndPersistsToTheDB(t *testing.T) {
	symbols := &fakeTrackedSymbols{tracked: []domain.Symbol{{Symbol: "SPY"}}}
	repo := &fakeDayVolumeRepo{}
	tracker := intraday.NewDayVolumeTracker()
	gateway := fakeDayVolumeGateway(func(_ context.Context, syms []string) map[string]int64 {
		return map[string]int64{"SPY": 5_468_232}
	})

	refreshDayVolumes(context.Background(), gateway, repo, symbols, tracker)

	got, ok := tracker.Get("SPY")
	if !ok || got != 5_468_232 {
		t.Fatalf("tracker SPY = %d ok=%v, want 5468232 true", got, ok)
	}
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if repo.saved["SPY"] != 5_468_232 {
		t.Fatalf("saved to db SPY = %d, want 5468232 (the refresh must persist, not just cache in memory)", repo.saved["SPY"])
	}
}

type fakeDayVolumeGateway func(ctx context.Context, symbols []string) map[string]int64

func (f fakeDayVolumeGateway) FetchDayVolumes(ctx context.Context, symbols []string) map[string]int64 {
	return f(ctx, symbols)
}
