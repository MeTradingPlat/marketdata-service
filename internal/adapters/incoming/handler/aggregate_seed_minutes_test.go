package handler

import (
	"context"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
)

type minutesSeedService struct {
	bar     dto.CandleBar
	minutes map[int64]int64
}

func (s minutesSeedService) GetCurrentCandle(context.Context, string, domain.Timeframe) (*dto.CandleBar, error) {
	bar := s.bar
	return &bar, nil
}

func (s minutesSeedService) GetCurrentCandleWithMinutes(context.Context, string, domain.Timeframe) (*dto.CandleBar, map[int64]int64, error) {
	bar := s.bar
	return &bar, s.minutes, nil
}

func TestAggregateWorker_ASeededMinuteCanStillBeCorrectedByALateTick(t *testing.T) {
	previous := aggregateCloseDelay
	aggregateCloseDelay = 24 * time.Hour
	defer func() { aggregateCloseDelay = previous }()
	now := time.Now().UTC()
	period := livecandles.FormingPeriodStart(now, domain.M15)
	first, second := period, period.Add(time.Minute)
	service := minutesSeedService{
		bar:     dto.CandleBar{Time: period.Unix(), Open: 10, High: 11, Low: 9, Close: 10, Volume: 300},
		minutes: map[int64]int64{first.Unix(): 100, second.Unix(): 200},
	}
	raw := livecandles.NewBroadcaster[domain.Candle]()
	hub := newCandleAggregateHub(raw, service)
	ch, cancel := subscribeToChan(hub, "AAPL", "M15", domain.M15)
	defer cancel()

	raw.Publish("AAPL", m1(first, 130))
	raw.Publish("AAPL", m1(period.Add(2*time.Minute), 50))

	bars := mustDrain(t, ch, 2)

	if bars[0].Volume != 330 || bars[1].Volume != 380 {
		t.Fatalf("volumes = %d, %d; want 330 (the late correction replaces the seeded 100 with 130) and 380", bars[0].Volume, bars[1].Volume)
	}
}
