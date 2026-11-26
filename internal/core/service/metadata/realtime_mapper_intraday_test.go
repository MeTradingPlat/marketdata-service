package metadata

import (
	"testing"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
)

func TestApplyIntraday(t *testing.T) {
	tests := []struct {
		name      string
		snap      domain.IntradaySnapshot
		wantPrice bool
	}{
		{"all zero", domain.IntradaySnapshot{DayVolume: 0}, false},
		{"all present", domain.IntradaySnapshot{
			DayVolume: 100, PreMarketVolume: 10, PostMarketVolume: 5,
			PrevClose: 99, Open: 100, High: 105, Low: 98,
			PreMarketClose: 99.5, PostMarketClose: 104, CurrentPrice: 103,
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out dto.FundamentalRealtime
			applyIntraday(&out, tt.snap)
			if out.DayVolume == nil || out.PreMarketVolume == nil || out.PostMarketVolume == nil {
				t.Fatal("volumes must always be non-nil")
			}
			ptrs := []*float64{out.PrevClose, out.Open, out.High, out.Low, out.PreMarketClose, out.PostMarketClose, out.LastPrice}
			for i, p := range ptrs {
				if (p != nil) != tt.wantPrice {
					t.Fatalf("price[%d] non-nil = %v, want %v", i, p != nil, tt.wantPrice)
				}
			}
		})
	}
}
