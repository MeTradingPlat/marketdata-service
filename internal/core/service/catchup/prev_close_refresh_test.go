package catchup

import (
	"context"
	"errors"
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

	upsertedCloses   map[string]float64
	upsertedAttempts []string
}

func newFakePrevCloseFundamentals(stale []string) *fakePrevCloseFundamentals {
	return &fakePrevCloseFundamentals{stale: stale}
}

func (f *fakePrevCloseFundamentals) GetSymbolsWithStalePrevClose(ctx context.Context, windowStart time.Time) ([]string, error) {
	return f.stale, f.staleErr
}

func (f *fakePrevCloseFundamentals) UpsertPrevCloseBatch(ctx context.Context, closes map[string]float64, attemptedOnly []string) error {
	f.upsertedCloses = closes
	f.upsertedAttempts = attemptedOnly
	return nil
}

func TestRefreshPrevClose_UpsertsFoundAndMarksMissingAsAttemptedInOneBatch(t *testing.T) {
	fundamentals := newFakePrevCloseFundamentals([]string{"AAPL", "ILLIQUID"})
	candles := &fakePrevCloseCandles{closes: map[string]float64{"AAPL": 190.5}}

	if err := RefreshPrevClose(context.Background(), candles, fundamentals, time.Now()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := fundamentals.upsertedCloses["AAPL"]; got != 190.5 {
		t.Errorf("AAPL upserted = %v, want 190.5", got)
	}
	if len(fundamentals.upsertedAttempts) != 1 || fundamentals.upsertedAttempts[0] != "ILLIQUID" {
		t.Errorf("attemptedOnly = %v, want [ILLIQUID]", fundamentals.upsertedAttempts)
	}
	if _, ok := fundamentals.upsertedCloses["ILLIQUID"]; ok {
		t.Error("ILLIQUID should not be in the closes map -- it has no close")
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
