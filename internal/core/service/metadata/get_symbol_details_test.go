package metadata

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	fundamentalscache "github.com/MeTradingPlat/marketdata-service/internal/core/service/fundamentals"
)

type fakeDetailsSymRepo struct {
	out.SymbolRepository
	syms []domain.Symbol
}

func (f *fakeDetailsSymRepo) TrackedWithVolume(ctx context.Context) ([]domain.Symbol, error) {
	return f.syms, nil
}

type fakeDetailsIntraday struct {
	in.GetIntradaySnapshotService
	snap domain.IntradaySnapshot
	err  error
}

func (f *fakeDetailsIntraday) GetSnapshot(ctx context.Context, symbol string) (domain.IntradaySnapshot, error) {
	return f.snap, f.err
}

func TestGetSymbolDetails(t *testing.T) {
	ctx := context.Background()
	symCache := NewSymbolsCache(&fakeDetailsSymRepo{syms: []domain.Symbol{{Symbol: "AAPL"}}}, nil)
	symCache.ReloadAll(ctx)
	fundCache := fundamentalscache.NewFundamentalsCache(nil, nil, nil)
	prev := 190.0
	future := time.Now().AddDate(0, 0, 7).Format("2006-01-02")
	fundCache.MergePrevClose(map[string]float64{"AAPL": prev}, nil)
	fundCache.MergeMarketMetrics([]domain.Fundamentals{{Symbol: "AAPL", NextEarningsDate: future}})

	svc := NewGetSymbolDetailsService(symCache, fundCache, &fakeDetailsIntraday{err: errors.New("no candles")})
	if _, err := svc.GetSymbolDetails(ctx, "MISSING"); err == nil {
		t.Fatal("expected error for missing symbol")
	}
	got, err := svc.GetSymbolDetails(ctx, "AAPL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.FundamentalData.PrevClose == nil || *got.FundamentalData.PrevClose != 190 {
		t.Fatalf("PrevClose = %v, want 190", got.FundamentalData.PrevClose)
	}
	if got.FundamentalData.NextEarningsDate != future || got.FundamentalData.DaysUntilEarnings == nil {
		t.Fatalf("earnings mismatch: %+v", got.FundamentalData)
	}
	fundCache.MergeMarketMetrics([]domain.Fundamentals{{Symbol: "AAPL", NextEarningsDate: "2020-01-01"}})
	past, _ := svc.GetSymbolDetails(ctx, "AAPL")
	if past.FundamentalData.NextEarningsDate != "" || past.FundamentalData.DaysUntilEarnings != nil {
		t.Fatalf("expected past earnings cleared, got %+v", past.FundamentalData)
	}
}
