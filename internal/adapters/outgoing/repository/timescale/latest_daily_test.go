package timescale

import (
	"reflect"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestLatestDailyBySymbol(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	tNewer := time.Date(2026, 1, 4, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		candles []domain.Candle
		want    []domain.Candle
	}{
		{name: "empty", candles: []domain.Candle{}, want: []domain.Candle{}},
		{
			name: "mixed symbols, timeframes, unordered timestamps, newer M1 ignored",
			candles: []domain.Candle{
				{Symbol: "MSFT", Timeframe: domain.D1, Timestamp: t1, Volume: 100},
				{Symbol: "AAPL", Timeframe: domain.D1, Timestamp: t2, Volume: 200},
				{Symbol: "AAPL", Timeframe: domain.M1, Timestamp: tNewer, Volume: 999},
				{Symbol: "MSFT", Timeframe: domain.D1, Timestamp: t3, Volume: 300},
				{Symbol: "AAPL", Timeframe: domain.D1, Timestamp: t1, Volume: 150},
				{Symbol: "GOOG", Timeframe: domain.H1, Timestamp: t3, Volume: 400},
			},
			want: []domain.Candle{
				{Symbol: "AAPL", Timeframe: domain.D1, Timestamp: t2, Volume: 200},
				{Symbol: "MSFT", Timeframe: domain.D1, Timestamp: t3, Volume: 300},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := latestDailyBySymbol(tt.candles)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
