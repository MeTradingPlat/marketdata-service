package main

import (
	"testing"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestLiveEligible(t *testing.T) {
	tests := []struct {
		name     string
		symbol   domain.Symbol
		expected bool
	}{
		{
			name:     "AAPL common stock kept",
			symbol:   domain.Symbol{Symbol: "AAPL", Description: "Apple Inc. - Common Stock"},
			expected: true,
		},
		{
			name:     "SNOW class A common stock kept",
			symbol:   domain.Symbol{Symbol: "SNOW", Description: "Snowflake Inc. Class A Common Stock"},
			expected: true,
		},
		{
			name:     "ABC/WS warrant out",
			symbol:   domain.Symbol{Symbol: "ABC/WS", Description: "ABC Acquisition Corp"},
			expected: false,
		},
		{
			name:     "XYZW warrant description out",
			symbol:   domain.Symbol{Symbol: "XYZW", Description: "XYZ Corp - Warrants"},
			expected: false,
		},
		{
			name:     "ABCU units description out",
			symbol:   domain.Symbol{Symbol: "ABCU", Description: "ABC Acquisition Corp - Units"},
			expected: false,
		},
		{
			name:     "ABCR rights description out",
			symbol:   domain.Symbol{Symbol: "ABCR", Description: "ABC Acquisition Corp - Rights"},
			expected: false,
		},
		{
			name:     "ETF kept",
			symbol:   domain.Symbol{Symbol: "SPY", IsEtf: true, Description: "SPDR S&P 500 ETF Trust"},
			expected: true,
		},
		{
			name:     "ET common units kept",
			symbol:   domain.Symbol{Symbol: "ET", Description: "Energy Transfer LP Common Units"},
			expected: true,
		},
		{
			name:     "BEP limited partnership units kept",
			symbol:   domain.Symbol{Symbol: "BEP", Description: "Brookfield Renewable Partners L.P. Limited Partnership Units"},
			expected: true,
		},
		{
			name:     "NEEpT corporate units kept",
			symbol:   domain.Symbol{Symbol: "NEEpT", Description: "NextEra Energy, Inc. 7.375% Corporate Units"},
			expected: true,
		},
		{
			name:     "SPAC units with /U suffix out",
			symbol:   domain.Symbol{Symbol: "GHLXU", Description: "Gores Holdings X, Inc. - Units"},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := liveEligible([]domain.Symbol{tc.symbol})
			got := len(result) == 1
			if got != tc.expected {
				t.Fatalf("liveEligible(%+v) got kept=%v, expected kept=%v", tc.symbol, got, tc.expected)
			}
		})
	}
}
