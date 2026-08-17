package domain

import (
	"math"
	"testing"
	"time"
)

func monthlyCandles(closesByMonth map[int]float64) []Candle {
	var candles []Candle
	for key, close := range closesByMonth {
		year := key / 100
		month := key % 100
		ts := time.Date(year, time.Month(month), 15, 0, 0, 0, 0, time.UTC)
		candles = append(candles, Candle{Timestamp: ts, Close: close})
	}
	return candles
}

// betaTwoSeries arma 60 meses de cierres con retornos de mercado variados y
// retornos del simbolo exactamente 2x los del mercado cada mes -- el beta
// de esa serie es 2.0 por construccion.
func betaTwoSeries() (symbol, market map[int]float64) {
	marketReturns := []float64{0.02, 0.03, 0.01, 0.04, -0.01, 0.02, 0.025, 0.015, -0.02, 0.03}
	symbol = make(map[int]float64)
	market = make(map[int]float64)
	mPrice, sPrice := 100.0, 100.0
	key := 202001
	for i := 0; i < 60; i++ {
		mPrice *= 1 + marketReturns[i%len(marketReturns)]
		sPrice *= 1 + 2*marketReturns[i%len(marketReturns)]
		market[key] = mPrice
		symbol[key] = sPrice
		key++
		if key%100 == 13 {
			key += 88
		}
	}
	return symbol, market
}

func TestMonthlyBeta(t *testing.T) {
	symbolSeries, marketSeries := betaTwoSeries()

	tests := []struct {
		name    string
		symbol  map[int]float64
		market  map[int]float64
		wantNil bool
		want    float64
	}{
		{
			name:   "beta 2 por construccion",
			symbol: symbolSeries,
			market: marketSeries,
			want:   2.0,
		},
		{
			name: "pocos meses solapados devuelve nil",
			symbol: map[int]float64{
				202001: 100, 202002: 104, 202003: 108, 202004: 112, 202005: 116, 202006: 120,
			},
			market: map[int]float64{
				202001: 100, 202002: 102, 202003: 104, 202004: 106, 202005: 108, 202006: 110,
			},
			wantNil: true,
		},
		{
			name:    "sin velas devuelve nil",
			symbol:  map[int]float64{},
			market:  map[int]float64{},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MonthlyBeta(monthlyCandles(tt.symbol), monthlyCandles(tt.market))
			if tt.wantNil {
				if got != nil {
					t.Fatalf("expected nil, got %v", *got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected beta, got nil")
			}
			if math.Abs(*got-tt.want) > 0.001 {
				t.Errorf("beta = %v, want %v", *got, tt.want)
			}
		})
	}
}
