package catchup

import (
	"context"
	"errors"
	"testing"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	fundamentalscache "github.com/MeTradingPlat/marketdata-service/internal/core/service/fundamentals"
)

type fakeMMGateway struct {
	out.MarketDataGateway
	items []domain.Fundamentals
	err   error
}

func (f *fakeMMGateway) MarketMetrics(ctx context.Context, symbols []string) ([]domain.Fundamentals, error) {
	return f.items, f.err
}

type fakeMMRepo struct {
	out.FundamentalsRepository
	upserted []domain.Fundamentals
	err      error
}

func (f *fakeMMRepo) UpsertMarketMetrics(ctx context.Context, fundamentals []domain.Fundamentals) error {
	if f.err != nil {
		return f.err
	}
	f.upserted = fundamentals
	return nil
}

func TestRefreshMarketMetrics(t *testing.T) {
	symRepo := &fakeDividendSymbols{tracked: []domain.Symbol{{Symbol: "AAPL"}}}
	cache := fundamentalscache.NewFundamentalsCache(nil, nil, nil)

	repoErr := &fakeMMRepo{}
	RefreshMarketMetrics(context.Background(), &fakeMMGateway{err: errors.New("500")}, symRepo, repoErr, cache)
	if len(repoErr.upserted) != 0 {
		t.Fatalf("expected no upsert on gateway error, got %v", repoErr.upserted)
	}

	repo := &fakeMMRepo{}
	RefreshMarketMetrics(context.Background(), &fakeMMGateway{items: []domain.Fundamentals{{Symbol: "AAPL", Beta: 1.5}}}, symRepo, repo, cache)
	if len(repo.upserted) != 1 || cache.GetBatch([]string{"AAPL"})["AAPL"].Beta != 1.5 {
		t.Fatalf("expected upsert and cache update, upserted=%v", repo.upserted)
	}
}
