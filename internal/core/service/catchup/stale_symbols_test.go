package catchup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
)

type fakeWatermarkRepo struct {
	out.CandleRepository
	watermarks map[string]time.Time
	err        error
}

func (f *fakeWatermarkRepo) GetWatermarksBatch(ctx context.Context, symbols []string, tf domain.Timeframe) (map[string]time.Time, error) {
	return f.watermarks, f.err
}

func TestFilterStaleSymbols(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	tracked := []domain.Symbol{
		{Symbol: "FRESH"},
		{Symbol: "STALE"},
		{Symbol: "NEVER_SEEN"},
	}
	repo := &fakeWatermarkRepo{watermarks: map[string]time.Time{
		"FRESH": now.Add(-24 * time.Hour),
		"STALE": now.Add(-20 * 24 * time.Hour),
	}}

	got := FilterStaleSymbols(context.Background(), repo, tracked, now)

	var symbols []string
	for _, s := range got {
		symbols = append(symbols, s.Symbol)
	}
	want := []string{"FRESH", "NEVER_SEEN"}
	if len(symbols) != len(want) {
		t.Fatalf("got %v, want %v", symbols, want)
	}
	for i, s := range want {
		if symbols[i] != s {
			t.Errorf("got %v, want %v", symbols, want)
		}
	}
}

func TestFilterStaleSymbols_RepositoryErrorKeepsEveryone(t *testing.T) {
	repo := &fakeWatermarkRepo{err: errors.New("boom")}
	tracked := []domain.Symbol{{Symbol: "A"}, {Symbol: "B"}}

	got := FilterStaleSymbols(context.Background(), repo, tracked, time.Now())

	if len(got) != 2 {
		t.Fatalf("expected all symbols kept on repository error, got %d", len(got))
	}
}
