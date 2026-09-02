package metadata

import (
	"context"
	"testing"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
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
