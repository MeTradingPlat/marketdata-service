package catchup

import (
	"context"
	"errors"
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

	upsertedVolumes  map[string]int64
	upsertedAttempts []string
}

func newFakePrevPostMarketVolumeFundamentals(stale []string) *fakePrevPostMarketVolumeFundamentals {
	return &fakePrevPostMarketVolumeFundamentals{stale: stale}
}

func (f *fakePrevPostMarketVolumeFundamentals) GetSymbolsWithStalePrevPostMarketVolume(ctx context.Context, windowStart time.Time) ([]string, error) {
	return f.stale, f.staleErr
}

func (f *fakePrevPostMarketVolumeFundamentals) UpsertPrevPostMarketVolumeBatch(ctx context.Context, volumes map[string]int64, attemptedOnly []string) error {
	f.upsertedVolumes = volumes
	f.upsertedAttempts = attemptedOnly
	return nil
}

func TestRefreshPrevPostMarketVolume_UpsertsFoundAndMarksMissingAsAttemptedInOneBatch(t *testing.T) {
	fundamentals := newFakePrevPostMarketVolumeFundamentals([]string{"AAPL", "NEWLISTING"})
	candles := &fakePrevPostMarketVolumeCandles{volumes: map[string]int64{"AAPL": 6600000}}

	if err := RefreshPrevPostMarketVolume(context.Background(), candles, fundamentals, time.Now()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := fundamentals.upsertedVolumes["AAPL"]; got != 6600000 {
		t.Errorf("AAPL upserted = %v, want 6600000", got)
	}
	if len(fundamentals.upsertedAttempts) != 1 || fundamentals.upsertedAttempts[0] != "NEWLISTING" {
		t.Errorf("attemptedOnly = %v, want [NEWLISTING]", fundamentals.upsertedAttempts)
	}
	if _, ok := fundamentals.upsertedVolumes["NEWLISTING"]; ok {
		t.Error("NEWLISTING should not be in the volumes map -- it has no volume")
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
