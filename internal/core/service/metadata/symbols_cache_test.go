package metadata

import (
	"context"
	"testing"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
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

func seedCache(t *testing.T, symbols []domain.Symbol) *SymbolsCache {
	t.Helper()
	c := NewSymbolsCache(&fakeSymbolRepo{symbols: symbols})
	c.ReloadAll(context.Background())
	return c
}
