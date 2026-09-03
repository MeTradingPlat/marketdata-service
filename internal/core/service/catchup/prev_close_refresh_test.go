package catchup

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
)

type fakePrevCloseCandles struct {
	out.CandleRepository
	closes map[string]float64
	err    error
}

func (f *fakePrevCloseCandles) GetPreviousSessionCloseBatch(ctx context.Context, symbols []string, before time.Time) (map[string]float64, error) {
	return f.closes, f.err
}

type fakePrevCloseFundamentals struct {
	out.FundamentalsRepository
	stale    []string
	staleErr error

	mu        sync.Mutex
	upserted  map[string]float64
	attempted map[string]bool
}

func newFakePrevCloseFundamentals(stale []string) *fakePrevCloseFundamentals {
	return &fakePrevCloseFundamentals{stale: stale, upserted: map[string]float64{}, attempted: map[string]bool{}}
}

func (f *fakePrevCloseFundamentals) GetSymbolsWithStalePrevClose(ctx context.Context, windowStart time.Time) ([]string, error) {
	return f.stale, f.staleErr
}

func (f *fakePrevCloseFundamentals) UpsertPrevClose(ctx context.Context, symbol string, close float64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upserted[symbol] = close
	return nil
}

func (f *fakePrevCloseFundamentals) MarkPrevCloseAttempted(ctx context.Context, symbol string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attempted[symbol] = true
	return nil
}

func TestRefreshPrevClose_UpsertsFoundAndMarksMissingAsAttempted(t *testing.T) {
	fundamentals := newFakePrevCloseFundamentals([]string{"AAPL", "ILLIQUID"})
	candles := &fakePrevCloseCandles{closes: map[string]float64{"AAPL": 190.5}}

	if err := RefreshPrevClose(context.Background(), candles, fundamentals, time.Now()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := fundamentals.upserted["AAPL"]; got != 190.5 {
		t.Errorf("AAPL upserted = %v, want 190.5", got)
	}
	if !fundamentals.attempted["ILLIQUID"] {
		t.Error("expected ILLIQUID (no close found) to be marked attempted")
	}
	if _, ok := fundamentals.upserted["ILLIQUID"]; ok {
		t.Error("ILLIQUID should not have been upserted -- it has no close")
	}
}

func TestRefreshPrevClose_NoStaleSymbolsSkipsBatchRead(t *testing.T) {
	fundamentals := newFakePrevCloseFundamentals(nil)
	candles := &fakePrevCloseCandles{err: errors.New("GetPreviousSessionCloseBatch should not be called")}

	if err := RefreshPrevClose(context.Background(), candles, fundamentals, time.Now()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRefreshPrevClose_BatchReadErrorPropagates(t *testing.T) {
	fundamentals := newFakePrevCloseFundamentals([]string{"AAPL"})
	candles := &fakePrevCloseCandles{err: errors.New("db down")}

	if err := RefreshPrevClose(context.Background(), candles, fundamentals, time.Now()); err == nil {
		t.Fatal("expected an error when the batch read fails")
	}
}
