package main

import (
	"context"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
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
	mu         sync.Mutex
	seed       map[string]int64
	boundaries out.DayBoundaryVolumes
	saved      map[string]int64
	savedDay   time.Time
	preSaved   map[string]int64
	regularEnd map[string]int64
}

func (f *fakeDayVolumeRepo) GetBoundaries(context.Context, []string, time.Time) (out.DayBoundaryVolumes, error) {
	return f.boundaries, nil
}
func (f *fakeDayVolumeRepo) SavePreMarketEnd(_ context.Context, _ time.Time, volumes map[string]int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.preSaved = volumes
	return nil
}
func (f *fakeDayVolumeRepo) SaveRegularEnd(_ context.Context, _ time.Time, volumes map[string]int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.regularEnd = volumes
	return nil
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

func TestSeedDayVolumesFromDB_AlsoRestoresTheSessionBoundaries(t *testing.T) {
	symbols := &fakeTrackedSymbols{tracked: []domain.Symbol{{Symbol: "SPY"}}}
	repo := &fakeDayVolumeRepo{
		seed:       map[string]int64{"SPY": 5_468_232},
		boundaries: out.DayBoundaryVolumes{PreMarketEnd: map[string]int64{"SPY": 480_000}, RegularEnd: map[string]int64{"SPY": 5_000_000}},
	}
	tracker := intraday.NewDayVolumeTracker()

	seedDayVolumesFromDB(context.Background(), repo, symbols, tracker)

	pre, okPre := tracker.Boundary("SPY", intraday.PreMarketEnd)
	regular, okRegular := tracker.Boundary("SPY", intraday.RegularEnd)
	if !okPre || pre != 480_000 || !okRegular || regular != 5_000_000 {
		t.Fatalf("pre=%d/%v regular=%d/%v, want 480000 and 5000000", pre, okPre, regular, okRegular)
	}
}
