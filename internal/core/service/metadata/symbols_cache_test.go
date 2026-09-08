package metadata

import (
	"context"
	"testing"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/intraday"
)

type fakeSymbolRepo struct {
	symbols []domain.Symbol
	err     error
}

func (f *fakeSymbolRepo) Upsert(context.Context, []domain.Symbol) error { return nil }
func (f *fakeSymbolRepo) Tracked(context.Context) ([]domain.Symbol, error) {
	return f.symbols, f.err
}
func (f *fakeSymbolRepo) TrackedWithVolume(context.Context) ([]domain.Symbol, error) {
	return f.symbols, f.err
}
func (f *fakeSymbolRepo) GetBySymbol(context.Context, string) (domain.Symbol, error) {
	return domain.Symbol{}, nil
}
func (f *fakeSymbolRepo) GetBatch(context.Context, []string) (map[string]domain.Symbol, error) {
	return nil, nil
}
func (f *fakeSymbolRepo) Search(context.Context, string, []string, int, int) ([]domain.Symbol, int64, error) {
	return nil, 0, nil
}
func (f *fakeSymbolRepo) Deactivate(context.Context, []string) error { return nil }
func (f *fakeSymbolRepo) Markets(context.Context) ([]string, error)  { return nil, nil }

// seedCache usa un SnapshotTracker vacio (nada operado hoy en ninguna
// prueba) -- todo cae al last_volume ya sembrado, sin sorpresas para las
// pruebas que solo les importa el orden/filtro estatico.
func seedCache(t *testing.T, symbols []domain.Symbol) *SymbolsCache {
	t.Helper()
	c := NewSymbolsCache(&fakeSymbolRepo{symbols: symbols}, intraday.NewSnapshotTracker())
	c.ReloadAll(context.Background())
	return c
}

// TestSymbolsCache_GetBatch_ServesFromMemory -- ver el comentario del
// metodo: sin el, GetFundamentalsRealtime pegaba directo a Postgres por
// esta parte pese a que fundamentales e intradia de esa misma llamada ya
// salian del cache.
func TestSymbolsCache_GetBatch_ServesFromMemory(t *testing.T) {
	c := seedCache(t, []domain.Symbol{
		{Symbol: "AAPL", Market: "XNAS", Description: "Apple Inc."},
		{Symbol: "MSFT", Market: "XNAS", Description: "Microsoft Corp."},
	})

	got, err := c.GetBatch(context.Background(), []string{"AAPL", "GOOG"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got["AAPL"].Description != "Apple Inc." {
		t.Fatalf("expected just AAPL with its cached data, got %+v", got)
	}
	if _, ok := got["GOOG"]; ok {
		t.Fatal("expected GOOG to be absent, not zero-valued")
	}
}
