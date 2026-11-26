package metadata

import (
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestToRealtime_IncludesNewFieldsAndNullVsZeroEps(t *testing.T) {
	now := time.Now()
	future := now.AddDate(0, 0, 10).Format("2006-01-02")
	oi := 4200.0
	zeroEps := 0.0

	f := domain.Fundamentals{
		Symbol:              "AAPL",
		TradingStatus:       "HALTED",
		MarketDataUpdatedAt: &now,
		MetricsUpdatedAt:    &now,
		Eps:                 &zeroEps,
		NextEarningsDate:    future,
		OpenInterest:        &oi,
	}
	snap := domain.IntradaySnapshot{Symbol: "AAPL", CurrentPrice: 195.5}

	got := toRealtime("AAPL", domain.Symbol{Symbol: "AAPL"}, f, snap)

	if got.LastPrice == nil || *got.LastPrice != 195.5 {
		t.Fatalf("LastPrice = %v, want 195.5", got.LastPrice)
	}
	if got.OpenInterest == nil || *got.OpenInterest != 4200 {
		t.Fatalf("OpenInterest = %v, want 4200", got.OpenInterest)
	}
	if got.NextEarningsDate == nil || *got.NextEarningsDate != future {
		t.Fatalf("NextEarningsDate = %v, want %s", got.NextEarningsDate, future)
	}
	if got.Eps == nil || *got.Eps != 0 {
		t.Fatalf("Eps = %v, want 0", got.Eps)
	}
	if got.TradingStatus == nil || *got.TradingStatus != "HALTED" {
		t.Fatalf("TradingStatus = %v, want HALTED", got.TradingStatus)
	}

	f.Eps = nil
	gotMissing := toRealtime("AAPL", domain.Symbol{Symbol: "AAPL"}, f, snap)
	if gotMissing.Eps != nil {
		t.Fatalf("Eps = %v, want nil when missing", *gotMissing.Eps)
	}
}
