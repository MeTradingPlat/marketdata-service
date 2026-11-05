package timescale

import (
	"slices"
	"testing"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestBySymbolSortsWithoutMutatingInput(t *testing.T) {
	in := []domain.Fundamentals{{Symbol: "MSFT"}, {Symbol: "AAPL"}, {Symbol: "GOOG"}}
	got := bySymbol(in)
	want := []string{"AAPL", "GOOG", "MSFT"}
	for i, s := range want {
		if got[i].Symbol != s {
			t.Fatalf("got[%d] = %s, want %s", i, got[i].Symbol, s)
		}
	}
	if in[0].Symbol != "MSFT" {
		t.Fatalf("input mutated: in[0] = %s", in[0].Symbol)
	}
}

func TestLockOrderSortsAndDedupsMapAndAttempted(t *testing.T) {
	m := map[string]int{"TSLA": 1, "AAPL": 2}
	attempted := []string{"MSFT", "AAPL"}
	got := lockOrder(m, attempted)
	want := []string{"AAPL", "MSFT", "TSLA"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
