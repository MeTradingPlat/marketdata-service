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

func TestAggregateWorker_UnaCorreccionTardiaDeUnPeriodoCerradoSeReenviaCorregida(t *testing.T) {
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

	bar := mustDrain(t, ch, 1)[0]
	if !bar.Closed || !bar.Corrected || bar.Volume != 80 || bar.Time != old.Unix() {
		t.Fatalf("debe reenviarse la MISMA vela cerrada con el volumen corregido: %+v", bar)
	}
}

func TestAggregateWorker_UnaCorreccionSinCambiosNoSeReenvia(t *testing.T) {
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

	raw.Publish("AAPL", m1(old, 70))

	select {
	case bar := <-ch:
		t.Fatalf("una repeticion identica no debe reenviarse: %+v", bar)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestAggregateWorker_CorregirUnMinutoAnteriorNoCambiaElCierreDeLaVela(t *testing.T) {
	previous := aggregateCloseDelay
	aggregateCloseDelay = time.Hour
	defer func() { aggregateCloseDelay = previous }()
	raw := livecandles.NewBroadcaster[domain.Candle]()
	hub := newCandleAggregateHub(raw, nilCurrentCandleService{})
	ch, cancel := subscribeToChan(hub, "AAPL", "M5", domain.M5)
	defer cancel()
	start := livecandles.FormingPeriodStart(time.Now().UTC(), domain.M5).Add(-5 * time.Minute)
	withClose := func(ts time.Time, closePrice float64, volume int64) domain.Candle {
		return domain.Candle{Symbol: "AAPL", Timestamp: ts, Open: 10, High: 12, Low: 9, Close: closePrice, Volume: volume}
	}
	raw.Publish("AAPL", withClose(start, 10.1, 100))
	raw.Publish("AAPL", withClose(start.Add(4*time.Minute), 10.9, 30))
	raw.Publish("AAPL", withClose(start.Add(5*time.Minute), 11, 5))
	mustDrain(t, ch, 4)

	raw.Publish("AAPL", withClose(start, 10.2, 120))

	corrected := mustDrain(t, ch, 1)[0]
	if corrected.Close != 10.9 || corrected.Volume != 150 || !corrected.Corrected {
		t.Fatalf("el cierre debe seguir siendo el del ultimo minuto y el volumen 120+30: %+v", corrected)
	}
}

func TestAggregateWorker_UnaCorreccionMasVieja_QueElUltimoPeriodoCerradoSeIgnora(t *testing.T) {
	previous := aggregateCloseDelay
	aggregateCloseDelay = time.Hour
	defer func() { aggregateCloseDelay = previous }()
	raw := livecandles.NewBroadcaster[domain.Candle]()
	hub := newCandleAggregateHub(raw, nilCurrentCandleService{})
	ch, cancel := subscribeToChan(hub, "AAPL", "M5", domain.M5)
	defer cancel()
	start := livecandles.FormingPeriodStart(time.Now().UTC(), domain.M5).Add(-10 * time.Minute)
	raw.Publish("AAPL", m1(start, 100))
	raw.Publish("AAPL", m1(start.Add(5*time.Minute), 10))
	raw.Publish("AAPL", m1(start.Add(10*time.Minute), 10))
	mustDrain(t, ch, 5)

	raw.Publish("AAPL", m1(start, 999))

	select {
	case bar := <-ch:
		t.Fatalf("solo se corrige el ultimo periodo cerrado, no uno anterior: %+v", bar)
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
