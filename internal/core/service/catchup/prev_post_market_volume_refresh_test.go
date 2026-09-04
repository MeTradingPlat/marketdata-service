package catchup

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
)

type fakePrevPostMarketVolumeCandles struct {
	out.CandleRepository
	volumes map[string]int64
	err     error
}

func (f *fakePrevPostMarketVolumeCandles) GetPreviousPostMarketVolumeBatch(ctx context.Context, symbols []string, before time.Time) (map[string]int64, error) {
	return f.volumes, f.err
}

type fakePrevPostMarketVolumeFundamentals struct {
	out.FundamentalsRepository
	stale    []string
	staleErr error

	mu        sync.Mutex
	upserted  map[string]int64
	attempted map[string]bool
}

func newFakePrevPostMarketVolumeFundamentals(stale []string) *fakePrevPostMarketVolumeFundamentals {
	return &fakePrevPostMarketVolumeFundamentals{stale: stale, upserted: map[string]int64{}, attempted: map[string]bool{}}
}

func (f *fakePrevPostMarketVolumeFundamentals) GetSymbolsWithStalePrevPostMarketVolume(ctx context.Context, windowStart time.Time) ([]string, error) {
	return f.stale, f.staleErr
}

func (f *fakePrevPostMarketVolumeFundamentals) UpsertPrevPostMarketVolume(ctx context.Context, symbol string, volume int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upserted[symbol] = volume
	return nil
}

func (f *fakePrevPostMarketVolumeFundamentals) MarkPrevPostMarketVolumeAttempted(ctx context.Context, symbol string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attempted[symbol] = true
	return nil
}

func TestRefreshPrevPostMarketVolume_UpsertsFoundAndMarksMissingAsAttempted(t *testing.T) {
	fundamentals := newFakePrevPostMarketVolumeFundamentals([]string{"AAPL", "NEWLISTING"})
	candles := &fakePrevPostMarketVolumeCandles{volumes: map[string]int64{"AAPL": 6600000}}

	if err := RefreshPrevPostMarketVolume(context.Background(), candles, fundamentals, time.Now()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := fundamentals.upserted["AAPL"]; got != 6600000 {
		t.Errorf("AAPL upserted = %v, want 6600000", got)
	}
	if !fundamentals.attempted["NEWLISTING"] {
		t.Error("expected NEWLISTING (no volume found) to be marked attempted")
	}
	if _, ok := fundamentals.upserted["NEWLISTING"]; ok {
		t.Error("NEWLISTING should not have been upserted -- it has no volume")
	}
}

func TestRefreshPrevPostMarketVolume_NoStaleSymbolsSkipsBatchRead(t *testing.T) {
	fundamentals := newFakePrevPostMarketVolumeFundamentals(nil)
	candles := &fakePrevPostMarketVolumeCandles{err: errors.New("GetPreviousPostMarketVolumeBatch should not be called")}

	if err := RefreshPrevPostMarketVolume(context.Background(), candles, fundamentals, time.Now()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRefreshPrevPostMarketVolume_BatchReadErrorPropagates(t *testing.T) {
	fundamentals := newFakePrevPostMarketVolumeFundamentals([]string{"AAPL"})
	candles := &fakePrevPostMarketVolumeCandles{err: errors.New("db down")}

	if err := RefreshPrevPostMarketVolume(context.Background(), candles, fundamentals, time.Now()); err == nil {
		t.Fatal("expected an error when the batch read fails")
	}
}
