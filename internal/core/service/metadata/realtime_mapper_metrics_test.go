package metadata

import (
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
)

func TestApplyMarketMetrics(t *testing.T) {
	future := time.Now().AddDate(0, 0, 5).Format("2006-01-02")
	zeroEps := 0.0
	tests := []struct {
		name string
		f    domain.Fundamentals
		want bool
	}{
		{"missing and past earnings", domain.Fundamentals{NextEarningsDate: "2020-01-01"}, false},
		{"all present", domain.Fundamentals{
			MarketCap: 1e9, Eps: &zeroEps, Beta: 1.2, Liquidity: 500, LiquidityRating: 4,
			ImpliedVolatilityIndex: 0.3, ImpliedVolatilityRank: 50, ImpliedVolatilityPercentile: 60,
			NextEarningsDate: future,
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out dto.FundamentalRealtime
			applyMarketMetrics(&out, tt.f, domain.IntradaySnapshot{})
			if out.LiquidityRating == nil {
				t.Fatal("LiquidityRating must always be set")
			}
			if (out.Eps != nil) != tt.want || (out.NextEarningsDate != nil) != tt.want || (out.DaysUntilEarnings != nil) != tt.want {
				t.Fatalf("eps/earnings presence mismatch: %+v", out)
			}
			ptrs := []*float64{out.MarketCap, out.Beta, out.Liquidity, out.ImpliedVolatilityIndex, out.ImpliedVolatilityRank, out.ImpliedVolatilityPercentile}
			for i, p := range ptrs {
				if (p != nil) != tt.want {
					t.Fatalf("metric[%d] non-nil = %v, want %v", i, p != nil, tt.want)
				}
			}
		})
	}
}
