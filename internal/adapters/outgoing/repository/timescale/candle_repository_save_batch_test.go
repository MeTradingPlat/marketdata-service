package timescale

import (
	"errors"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/jackc/pgx/v5/pgconn"
)

type fakeBatchResultExecer struct {
	tags     []pgconn.CommandTag
	err      error
	closeErr error
	idx      int
}

func (f *fakeBatchResultExecer) Close() error { return f.closeErr }

func (f *fakeBatchResultExecer) Exec() (pgconn.CommandTag, error) {
	if f.err != nil && f.idx == len(f.tags) {
		return pgconn.CommandTag{}, f.err
	}
	tag := f.tags[f.idx]
	f.idx++
	return tag, nil
}

func TestBuildCandleSaveBatch(t *testing.T) {
	ts := time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC)
	candles := []domain.Candle{
		{Symbol: "AAPL", Timeframe: domain.M1, Timestamp: ts},
		{Symbol: "AAPL", Timeframe: domain.D1, Timestamp: ts},
	}
	dailies := []domain.Candle{{Symbol: "AAPL", Timeframe: domain.D1, Timestamp: ts}}

	if got := buildCandleSaveBatch(candles, dailies, true).Len(); got != 5 {
		t.Fatalf("withWatermark=true Len() = %d, want 5", got)
	}
	if got := buildCandleSaveBatch(candles, dailies, false).Len(); got != 3 {
		t.Fatalf("withWatermark=false Len() = %d, want 3", got)
	}
}

func TestReadCandleBatchResults(t *testing.T) {
	okTag := pgconn.NewCommandTag("INSERT 0 1")
	execer := &fakeBatchResultExecer{tags: []pgconn.CommandTag{okTag, okTag, okTag}}
	affected, err := readCandleBatchResults(execer, 1, 1, true)
	if err != nil || affected != 1 {
		t.Fatalf("got (%d, %v), want (1, nil)", affected, err)
	}

	execErr := errors.New("boom")
	errExecer := &fakeBatchResultExecer{tags: []pgconn.CommandTag{okTag}, err: execErr}
	if _, err := readCandleBatchResults(errExecer, 1, 0, true); err == nil {
		t.Fatal("expected error when watermark exec fails")
	}

	closeErrExecer := &fakeBatchResultExecer{tags: []pgconn.CommandTag{okTag}, closeErr: errors.New("close boom")}
	if _, err := consumeCandleBatchResults(closeErrExecer, 1, 0, false); err == nil {
		t.Fatal("expected error when Close fails")
	}

	bothErrExecer := &fakeBatchResultExecer{tags: []pgconn.CommandTag{okTag}, err: execErr, closeErr: errors.New("close boom")}
	if _, err := consumeCandleBatchResults(bothErrExecer, 1, 0, true); !errors.Is(err, execErr) {
		t.Fatalf("expected exec error to be preserved, got %v", err)
	}
}
