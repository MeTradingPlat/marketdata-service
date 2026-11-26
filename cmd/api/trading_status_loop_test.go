package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/fundamentals"
)

type fakeTSGateway struct {
	out.MarketDataGateway
	err error
}

func (f *fakeTSGateway) DividendInfo(ctx context.Context, symbols []string) ([]domain.Fundamentals, error) {
	return []domain.Fundamentals{{Symbol: "AAPL"}}, f.err
}

type fakeTSSymbols struct{ out.SymbolRepository }

func (f *fakeTSSymbols) Tracked(ctx context.Context) ([]domain.Symbol, error) {
	return []domain.Symbol{{Symbol: "AAPL"}}, nil
}

type fakeTSFunds struct{ out.FundamentalsRepository }

func (f *fakeTSFunds) UpsertDividends(ctx context.Context, _ []domain.Fundamentals) error { return nil }

func TestIsMarketActiveWindow(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	tests := []struct {
		name string
		t    time.Time
		want bool
	}{
		{"before 4am ET", time.Date(2026, 11, 25, 3, 59, 0, 0, loc), false},
		{"at 4am ET", time.Date(2026, 11, 25, 4, 0, 0, 0, loc), true},
		{"midday ET", time.Date(2026, 11, 25, 12, 0, 0, 0, loc), true},
		{"at 8pm ET", time.Date(2026, 11, 25, 20, 0, 0, 0, loc), false},
		{"saturday", time.Date(2026, 11, 28, 12, 0, 0, 0, loc), false},
		{"sunday", time.Date(2026, 11, 29, 12, 0, 0, 0, loc), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMarketActiveWindow(tt.t); got != tt.want {
				t.Fatalf("isMarketActiveWindow(%v) = %v, want %v", tt.t, got, tt.want)
			}
		})
	}
}

func TestRefreshTradingStatusStep(t *testing.T) {
	cache := fundamentals.NewFundamentalsCache(nil, nil, nil)
	lastTradingStatusAtUnix.Store(0)
	refreshTradingStatusStep(context.Background(), &fakeTSGateway{err: errors.New("500")}, &fakeTSSymbols{}, &fakeTSFunds{}, cache)
	if lastTradingStatusAtUnix.Load() != 0 {
		t.Fatal("lastTradingStatusAtUnix must not update on error")
	}
	refreshTradingStatusStep(context.Background(), &fakeTSGateway{}, &fakeTSSymbols{}, &fakeTSFunds{}, cache)
	if lastTradingStatusAtUnix.Load() == 0 {
		t.Fatal("lastTradingStatusAtUnix must update on success")
	}
}
