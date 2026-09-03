package metadata

import (
	"context"
	"errors"
	"testing"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/intraday"
)

func TestSymbolsCache_ReloadAll_SortsByVolumeThenSymbol(t *testing.T) {
	c := seedCache(t, []domain.Symbol{
		{Symbol: "AAPL", Market: "XNAS", LastVolume: 100},
		{Symbol: "MSFT", Market: "XNAS", LastVolume: 500},
		{Symbol: "ZZZZ", Market: "XNAS", LastVolume: 500},
	})

	tracked, err := c.Tracked(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"MSFT", "ZZZZ", "AAPL"}
	for i, s := range tracked {
		if s.Symbol != want[i] {
			t.Errorf("position %d = %s, want %s (order: %v)", i, s.Symbol, want[i], symbolNames(tracked))
		}
	}
}

func TestSymbolsCache_ReloadAll_KeepsPreviousDataOnError(t *testing.T) {
	repo := &fakeSymbolRepo{symbols: []domain.Symbol{{Symbol: "AAPL"}}}
	c := NewSymbolsCache(repo, intraday.NewSnapshotTracker())
	c.ReloadAll(context.Background())

	repo.err = errors.New("db down")
	repo.symbols = nil
	c.ReloadAll(context.Background())

	tracked, _ := c.Tracked(context.Background())
	if len(tracked) != 1 || tracked[0].Symbol != "AAPL" {
		t.Fatalf("expected previous data to survive a failed reload, got %v", tracked)
	}
}

func TestSymbolsCache_Markets_DedupsAndSorts(t *testing.T) {
	c := seedCache(t, []domain.Symbol{
		{Symbol: "AAPL", Market: "XNAS"},
		{Symbol: "IBM", Market: "XNYS"},
		{Symbol: "MSFT", Market: "XNAS"},
	})

	markets, err := c.Markets(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"XNAS", "XNYS"}
	if len(markets) != len(want) || markets[0] != want[0] || markets[1] != want[1] {
		t.Errorf("Markets() = %v, want %v", markets, want)
	}
}

func TestSymbolsCache_GetBySymbol(t *testing.T) {
	c := seedCache(t, []domain.Symbol{{Symbol: "AAPL", Market: "XNAS"}})

	if _, err := c.GetBySymbol(context.Background(), "MISSING"); err == nil {
		t.Error("expected an error for an untracked symbol")
	}
	got, err := c.GetBySymbol(context.Background(), "AAPL")
	if err != nil || got.Symbol != "AAPL" {
		t.Errorf("GetBySymbol(AAPL) = %+v, %v", got, err)
	}
}

func symbolNames(symbols []domain.Symbol) []string {
	names := make([]string, len(symbols))
	for i, s := range symbols {
		names[i] = s.Symbol
	}
	return names
}
