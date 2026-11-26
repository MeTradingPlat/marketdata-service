package catchup

import (
	"context"
	"errors"
	"testing"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	fundamentalscache "github.com/MeTradingPlat/marketdata-service/internal/core/service/fundamentals"
)

type fakeDividendSymbols struct {
	out.SymbolRepository
	tracked []domain.Symbol
}

func (f *fakeDividendSymbols) Tracked(ctx context.Context) ([]domain.Symbol, error) {
	return f.tracked, nil
}

type fakeDividendGateway struct {
	out.MarketDataGateway
	err error
}

func (f *fakeDividendGateway) DividendInfo(ctx context.Context, symbols []string) ([]domain.Fundamentals, error) {
	if f.err != nil {
		return nil, f.err
	}
	return []domain.Fundamentals{{Symbol: "AAPL", TradingStatus: "ACTIVE"}}, nil
}

type fakeDividendFundamentals struct {
	out.FundamentalsRepository
}

func (f *fakeDividendFundamentals) UpsertDividends(ctx context.Context, fundamentals []domain.Fundamentals) error {
	return nil
}

func TestRefreshTradingStatus_ReturnsErrorOnGatewayFailure(t *testing.T) {
	symRepo := &fakeDividendSymbols{tracked: []domain.Symbol{{Symbol: "AAPL"}}}
	fundRepo := &fakeDividendFundamentals{}
	cache := fundamentalscache.NewFundamentalsCache(nil, nil, nil)

	if err := RefreshTradingStatus(context.Background(), &fakeDividendGateway{err: errors.New("401")}, symRepo, fundRepo, cache); err == nil {
		t.Fatal("expected error when DividendInfo fails")
	}
	if err := RefreshTradingStatus(context.Background(), &fakeDividendGateway{}, symRepo, fundRepo, cache); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
