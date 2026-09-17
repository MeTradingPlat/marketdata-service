package handler

import (
	"context"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
)

type nilCurrentCandleService struct{}

func (nilCurrentCandleService) GetCurrentCandle(context.Context, string, domain.Timeframe) (*dto.CandleBar, error) {
	return nil, nil
}

func mustDrain(t *testing.T, ch <-chan dto.CandleBar, want int) []dto.CandleBar {
	t.Helper()
	var got []dto.CandleBar
	for len(got) < want {
		select {
		case bar := <-ch:
			got = append(got, bar)
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for bar %d/%d (got %d so far)", len(got)+1, want, len(got))
		}
	}
	return got
}

// subscribeToChan adapta el Subscribe basado en callback a un canal para que
// los tests puedan seguir usando mustDrain -- mismo patron que usa
// production (wsSession.seedAndSubscribe), solo que aca el push encola en un
// canal de test en vez de publicar a la sesion WS.
func subscribeToChan(hub *candleAggregateHub, symbol, timeframe string, tf domain.Timeframe) (<-chan dto.CandleBar, func()) {
	ch := make(chan dto.CandleBar, 16)
	cancel := hub.Subscribe(context.Background(), symbol, timeframe, tf, func(bar dto.CandleBar) { ch <- bar })
	return ch, cancel
}

func TestCandleAggregateHub_AggregatesM1IntoRequestedTimeframe(t *testing.T) {
	raw := livecandles.NewBroadcaster[domain.Candle]()
	hub := newCandleAggregateHub(raw, nilCurrentCandleService{})

	ch, cancel := subscribeToChan(hub, "AAPL", "M5", domain.M5)
	defer cancel()

	base := time.Date(2026, 8, 21, 14, 35, 0, 0, time.UTC)
	raw.Publish("AAPL", domain.Candle{Symbol: "AAPL", Timestamp: base, Open: 10, High: 10, Low: 9, Close: 9.5, Volume: 100})
	raw.Publish("AAPL", domain.Candle{Symbol: "AAPL", Timestamp: base.Add(time.Minute), Open: 9.5, High: 11, Low: 9.5, Close: 10.5, Volume: 50})
	// Nuevo periodo M5 -- cierra el anterior con Closed=true y arranca el siguiente en formacion.
	raw.Publish("AAPL", domain.Candle{Symbol: "AAPL", Timestamp: base.Add(5 * time.Minute), Open: 20, High: 20, Low: 20, Close: 20, Volume: 10})

	bars := mustDrain(t, ch, 3)

	if bars[0].Closed || bars[1].Closed {
		t.Fatalf("primeras dos barras no deberian venir cerradas: %+v", bars[:2])
	}
	if bars[1].High != 11 || bars[1].Low != 9 || bars[1].Close != 10.5 || bars[1].Volume != 150 {
		t.Fatalf("agregado M5 incorrecto tras el segundo tick: %+v", bars[1])
	}
	if !bars[2].Closed {
		t.Fatalf("la barra emitida al cruzar de periodo debe venir cerrada: %+v", bars[2])
	}
	if bars[2].High != 11 || bars[2].Volume != 150 {
		t.Fatalf("la barra cerrada debe ser la del periodo ANTERIOR, no la nueva: %+v", bars[2])
	}
}

func TestCandleAggregateHub_CompartelaAgregacionEntreSuscriptores(t *testing.T) {
	raw := livecandles.NewBroadcaster[domain.Candle]()
	hub := newCandleAggregateHub(raw, nilCurrentCandleService{})

	ch1, cancel1 := subscribeToChan(hub, "AAPL", "M5", domain.M5)
	defer cancel1()
	ch2, cancel2 := subscribeToChan(hub, "AAPL", "M5", domain.M5)
	defer cancel2()

	hub.mu.Lock()
	workers := len(hub.workers)
	refCount := hub.workers["AAPL:M5"].refCount
	hub.mu.Unlock()
	if workers != 1 {
		t.Fatalf("dos suscriptores al mismo (symbol, timeframe) deben compartir UN solo worker, hay %d", workers)
	}
	if refCount != 2 {
		t.Fatalf("refCount = %d, want 2", refCount)
	}

	base := time.Date(2026, 8, 21, 14, 35, 0, 0, time.UTC)
	raw.Publish("AAPL", domain.Candle{Symbol: "AAPL", Timestamp: base, Open: 10, High: 10, Low: 9, Close: 9.5, Volume: 100})

	bars1 := mustDrain(t, ch1, 1)
	bars2 := mustDrain(t, ch2, 1)
	if bars1[0] != bars2[0] {
		t.Fatalf("ambos suscriptores deben recibir el mismo agregado: %+v vs %+v", bars1[0], bars2[0])
	}
}

func TestCandleAggregateHub_ApagaElWorkerCuandoSeVaElUltimoSuscriptor(t *testing.T) {
	raw := livecandles.NewBroadcaster[domain.Candle]()
	hub := newCandleAggregateHub(raw, nilCurrentCandleService{})

	_, cancel1 := subscribeToChan(hub, "AAPL", "M5", domain.M5)
	_, cancel2 := subscribeToChan(hub, "AAPL", "M5", domain.M5)

	cancel1()
	hub.mu.Lock()
	stillThere := len(hub.workers)
	hub.mu.Unlock()
	if stillThere != 1 {
		t.Fatalf("el worker debe seguir vivo mientras quede al menos un suscriptor, workers=%d", stillThere)
	}

	cancel2()
	hub.mu.Lock()
	afterLast := len(hub.workers)
	hub.mu.Unlock()
	if afterLast != 0 {
		t.Fatalf("el worker debe apagarse cuando se va el ultimo suscriptor, workers=%d", afterLast)
	}
}
