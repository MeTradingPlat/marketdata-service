package handler

import (
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
)

func TestSeedAggregate_ConvertsEndStampedSeedToPeriodStart(t *testing.T) {
	now := time.Date(2026, 8, 21, 14, 37, 0, 0, time.UTC)
	periodEnd := time.Date(2026, 8, 21, 14, 40, 0, 0, time.UTC) // GetCurrentCandle stamps the seed with the period's end
	seed := &dto.CandleBar{Time: periodEnd.Unix(), Open: 1, High: 2, Low: 0.5, Close: 1.5, Volume: 100}

	agg := seedAggregate(seed, domain.M5, now)

	wantStart := time.Date(2026, 8, 21, 14, 35, 0, 0, time.UTC).Unix()
	if agg.Time != wantStart {
		t.Fatalf("agg.Time = %d, want %d (period start, not the seed's end-stamped time)", agg.Time, wantStart)
	}
	if agg.Open != 1 || agg.High != 2 || agg.Low != 0.5 || agg.Close != 1.5 || agg.Volume != 100 {
		t.Fatalf("OHLC/volume not preserved from seed: %+v", agg)
	}
}

func TestSeedAggregate_NilSeedReturnsNil(t *testing.T) {
	if seedAggregate(nil, domain.M5, time.Now()) != nil {
		t.Fatal("expected nil for a nil seed")
	}
}
