package metadata

import (
	"context"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/intraday"
)

func searchFixture(t *testing.T) *SymbolsCache {
	return seedCache(t, []domain.Symbol{
		{Symbol: "AAPL", Market: "XNAS", Description: "Apple Inc.", LastVolume: 900},
		{Symbol: "MSFT", Market: "XNAS", Description: "Microsoft Corp.", LastVolume: 800},
		{Symbol: "IBM", Market: "XNYS", Description: "International Business Machines", LastVolume: 100},
	})
}

func TestSymbolsCache_Search_FiltersByQueryAcrossSymbolAndDescription(t *testing.T) {
	c := searchFixture(t)

	results, total, _ := c.Search(context.Background(), "apple", nil, 0, 10)
	if total != 1 || len(results) != 1 || results[0].Symbol != "AAPL" {
		t.Errorf("query 'apple' = %v (total %d), want just AAPL", results, total)
	}

	results, total, _ = c.Search(context.Background(), "corp", nil, 0, 10)
	if total != 1 || results[0].Symbol != "MSFT" {
		t.Errorf("query 'corp' = %v (total %d), want just MSFT", results, total)
	}
}

func TestSymbolsCache_Search_FiltersByMarketCaseInsensitive(t *testing.T) {
	c := searchFixture(t)

	results, total, _ := c.Search(context.Background(), "", []string{"xnys"}, 0, 10)
	if total != 1 || results[0].Symbol != "IBM" {
		t.Errorf("market filter 'xnys' = %v (total %d), want just IBM", results, total)
	}
}

func TestSymbolsCache_Search_Paginates(t *testing.T) {
	c := searchFixture(t)

	page0, total, _ := c.Search(context.Background(), "", nil, 0, 2)
	if total != 3 || len(page0) != 2 {
		t.Fatalf("page 0 = %v (total %d), want 2 of 3", page0, total)
	}
	page1, _, _ := c.Search(context.Background(), "", nil, 1, 2)
	if len(page1) != 1 {
		t.Fatalf("page 1 = %v, want 1 remaining result", page1)
	}
}

func TestSymbolsCache_Search_RanksByTodayVolumeOverLastVolume(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	tracker := intraday.NewSnapshotTracker()
	preMarket := time.Date(2026, 1, 15, 7, 0, 0, 0, loc)
	tracker.RecordClosedCandle(domain.Candle{Symbol: "IBM", Timestamp: preMarket, Volume: 5000})

	repo := &fakeSymbolRepo{symbols: []domain.Symbol{
		{Symbol: "AAPL", Market: "XNAS", LastVolume: 900},
		{Symbol: "IBM", Market: "XNYS", LastVolume: 100},
	}}
	c := NewSymbolsCache(repo, tracker)
	c.ReloadAll(context.Background())

	results, _, err := c.Search(context.Background(), "", nil, 0, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 || results[0].Symbol != "IBM" {
		t.Fatalf("expected IBM (5000 today) ranked ahead of AAPL (900 last known, 0 today), got %v", results)
	}
}

func TestSymbolsCache_Search_PageBeyondResultsIsEmptyNotNil(t *testing.T) {
	c := searchFixture(t)

	results, total, err := c.Search(context.Background(), "", nil, 5, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results == nil || len(results) != 0 || total != 3 {
		t.Errorf("Search() past the end = %v (total %d), want empty slice, total 3", results, total)
	}
}
