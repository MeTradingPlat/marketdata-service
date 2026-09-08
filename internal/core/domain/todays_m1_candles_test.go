package domain_test

import (
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestTodaysM1Candles(t *testing.T) {
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)

	candles := []domain.Candle{
		{Symbol: "AAPL", Timeframe: domain.M1, Timestamp: now, Volume: 1},
		{Symbol: "AAPL", Timeframe: domain.M1, Timestamp: yesterday, Volume: 2},
		{Symbol: "AAPL", Timeframe: domain.D1, Timestamp: now, Volume: 3},
	}

	got := domain.TodaysM1Candles(candles, now)

	if len(got) != 1 || got[0].Volume != 1 {
		t.Fatalf("expected only today's M1 candle to survive, got %+v", got)
	}
}
