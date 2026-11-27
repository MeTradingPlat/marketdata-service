package intraday

import (
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestMergeFormingCandleAt(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		now         time.Time
		candle      domain.Candle
		wantPreVol  int64
		wantDayVol  int64
		wantPostVol int64
		wantHigh    float64
	}{
		{
			name:        "early close 13:30 goes to post",
			now:         time.Date(2026, 11, 27, 13, 30, 0, 0, loc),
			candle:      domain.Candle{Timestamp: time.Date(2026, 11, 27, 13, 30, 0, 0, loc), High: 120, Close: 112, Volume: 300},
			wantPostVol: 300,
		},
		{
			name:   "early close 02:00 ignored",
			now:    time.Date(2026, 11, 27, 2, 0, 0, 0, loc),
			candle: domain.Candle{Timestamp: time.Date(2026, 11, 27, 2, 0, 0, 0, loc), High: 120, Close: 112, Volume: 100},
		},
		{
			name:   "early close 17:30 ignored",
			now:    time.Date(2026, 11, 27, 17, 30, 0, 0, loc),
			candle: domain.Candle{Timestamp: time.Date(2026, 11, 27, 17, 30, 0, 0, loc), High: 120, Close: 112, Volume: 400},
		},
		{
			name:       "normal day 13:30 goes to regular",
			now:        time.Date(2026, 11, 30, 13, 30, 0, 0, loc),
			candle:     domain.Candle{Timestamp: time.Date(2026, 11, 30, 13, 30, 0, 0, loc), Open: 100, High: 120, Low: 99, Close: 112, Volume: 300},
			wantDayVol: 300,
			wantHigh:   120,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var snap domain.IntradaySnapshot
			mergeFormingCandleAt(&snap, tt.candle, tt.now)
			if snap.PreMarketVolume != tt.wantPreVol || snap.DayVolume != tt.wantDayVol ||
				snap.PostMarketVolume != tt.wantPostVol || snap.High != tt.wantHigh {
				t.Fatalf("got %+v, want pre=%d day=%d post=%d high=%v",
					snap, tt.wantPreVol, tt.wantDayVol, tt.wantPostVol, tt.wantHigh)
			}
		})
	}
}
