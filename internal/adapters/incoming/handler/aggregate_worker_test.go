package handler

import (
	"context"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
)

func m1(ts time.Time, volume int64) domain.Candle {
	return domain.Candle{Symbol: "AAPL", Timestamp: ts, Open: 10, High: 11, Low: 9, Close: 10, Volume: volume}
}

func TestAggregateWorker_LosTicksEnFormacionNoInflanElVolumen(t *testing.T) {
	raw := livecandles.NewBroadcaster[domain.Candle]()
	hub := newCandleAggregateHub(raw, nilCurrentCandleService{})
	ch, cancel := subscribeToChan(hub, "AAPL", "M5", domain.M5)
	defer cancel()
	base := time.Date(2026, 8, 21, 14, 35, 0, 0, time.UTC)

	raw.Publish("AAPL", m1(base, 40))
	raw.Publish("AAPL", m1(base, 100))
	raw.Publish("AAPL", m1(base, 100))
	raw.Publish("AAPL", m1(base.Add(time.Minute), 30))

	bars := mustDrain(t, ch, 4)

	if bars[3].Volume != 130 {
		t.Fatalf("volumen M5 = %d, want 130 (100 del primer minuto + 30 del segundo, sin sumar los ticks acumulados)", bars[3].Volume)
	}
}

func TestAggregateWorker_CierraLaVelaPorTiempoSinEsperarElSiguienteTick(t *testing.T) {
	previous := aggregateCloseDelay
	aggregateCloseDelay = 50 * time.Millisecond
	defer func() { aggregateCloseDelay = previous }()
	raw := livecandles.NewBroadcaster[domain.Candle]()
	hub := newCandleAggregateHub(raw, nilCurrentCandleService{})
	ch, cancel := subscribeToChan(hub, "AAPL", "M1", domain.M1)
	defer cancel()

	raw.Publish("AAPL", m1(time.Now().UTC().Truncate(time.Minute).Add(-time.Minute), 70))

	bars := mustDrain(t, ch, 2)

	if !bars[1].Closed || bars[1].Volume != 70 {
		t.Fatalf("la vela debio cerrarse sola por reloj con su volumen final: %+v", bars[1])
	}
}

func TestAggregateWorker_UnTickTardioDeUnPeriodoCerradoSeIgnora(t *testing.T) {
	previous := aggregateCloseDelay
	aggregateCloseDelay = 50 * time.Millisecond
	defer func() { aggregateCloseDelay = previous }()
	raw := livecandles.NewBroadcaster[domain.Candle]()
	hub := newCandleAggregateHub(raw, nilCurrentCandleService{})
	ch, cancel := subscribeToChan(hub, "AAPL", "M1", domain.M1)
	defer cancel()
	old := time.Now().UTC().Truncate(time.Minute).Add(-time.Minute)
	raw.Publish("AAPL", m1(old, 70))
	mustDrain(t, ch, 2)

	raw.Publish("AAPL", m1(old, 80))

	select {
	case bar := <-ch:
		t.Fatalf("un tick tardio de un periodo ya cerrado no debe reabrirlo: %+v", bar)
	case <-time.After(300 * time.Millisecond):
	}
}

type seededCurrentCandleService struct{ bar dto.CandleBar }

func (s seededCurrentCandleService) GetCurrentCandle(context.Context, string, domain.Timeframe) (*dto.CandleBar, error) {
	bar := s.bar
	return &bar, nil
}

func TestAggregateWorker_ElArranqueSembradoNoSumaDeNuevoElVolumenParcialDelMinuto(t *testing.T) {
	now := time.Now().UTC()
	period := livecandles.FormingPeriodStart(now, domain.M5).Unix()
	current := seededCurrentCandleService{bar: dto.CandleBar{Time: period, Open: 10, High: 11, Low: 9, Close: 10, Volume: 500}}
	raw := livecandles.NewBroadcaster[domain.Candle]()
	hub := newCandleAggregateHub(raw, current)
	ch, cancel := subscribeToChan(hub, "AAPL", "M5", domain.M5)
	defer cancel()
	minute := now.Truncate(time.Minute)

	raw.Publish("AAPL", m1(minute, 40))
	raw.Publish("AAPL", m1(minute, 70))

	bars := mustDrain(t, ch, 2)

	if bars[0].Volume != 500 || bars[1].Volume != 530 {
		t.Fatalf("volumenes = %d, %d; want 500 (adopta el primer tick como base) y 530", bars[0].Volume, bars[1].Volume)
	}
}

func TestAggregateWorker_UnaCorreccionTardiaDentroDelPeriodoAbiertoReemplazaSuMinuto(t *testing.T) {
	previous := aggregateCloseDelay
	aggregateCloseDelay = time.Hour
	defer func() { aggregateCloseDelay = previous }()
	raw := livecandles.NewBroadcaster[domain.Candle]()
	hub := newCandleAggregateHub(raw, nilCurrentCandleService{})
	ch, cancel := subscribeToChan(hub, "AAPL", "M5", domain.M5)
	defer cancel()
	start := livecandles.FormingPeriodStart(time.Now().UTC(), domain.M5)

	raw.Publish("AAPL", m1(start, 100))
	raw.Publish("AAPL", m1(start.Add(time.Minute), 30))
	raw.Publish("AAPL", m1(start, 120))
	raw.Publish("AAPL", m1(start.Add(time.Minute), 45))

	bars := mustDrain(t, ch, 4)

	if bars[2].Volume != 150 || bars[3].Volume != 165 {
		t.Fatalf("volumenes = %d, %d; want 150 (120+30 tras corregir el primer minuto) y 165 (120+45)", bars[2].Volume, bars[3].Volume)
	}
}
