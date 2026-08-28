package tastytrade

import (
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func newTestPool() *CandlePool {
	return &CandlePool{
		liveSubs:   make(map[string]func(domain.Candle)),
		liveTicks:  make(map[string]func(domain.Candle)),
		current:    make(map[string]domain.Candle),
		lastClosed: make(map[string]domain.Candle),
	}
}

func f(v float64) *float64 { return &v }

// Regression: un tick tardio para una vela YA cerrada (el reporte del
// exchange/SIP llego despues de que ya cerramos el minuto siguiente) no
// debe confundirse con "una vela nueva" -- confirmado en vivo el
// 2026-08-27/28: cerraba de golpe la vela en formacion REAL con datos a
// medio completar y reabria la vieja como si fuera la actual.
func TestHandleLiveEvent_LateTickDoesNotDisturbCurrentForming(t *testing.T) {
	p := newTestPool()
	minute10 := time.Date(2026, 8, 27, 18, 10, 0, 0, time.UTC)
	minute11 := time.Date(2026, 8, 27, 18, 11, 0, 0, time.UTC)
	minute12 := time.Date(2026, 8, 27, 18, 12, 0, 0, time.UTC)

	var closedEvents []domain.Candle
	var tickEvents []domain.Candle
	p.liveSubs["EMAT"] = func(c domain.Candle) { closedEvents = append(closedEvents, c) }
	p.liveTicks["EMAT"] = func(c domain.Candle) { tickEvents = append(tickEvents, c) }

	// 18:10 en formacion, un tick.
	p.handleLiveEvent("EMAT", rawCandleEvent{Symbol: "EMAT", Timestamp: minute10, Open: f(3.18), High: f(3.18), Low: f(3.18), Close: f(3.18), Volume: f(200)})
	// Llega el primer tick de 18:11 -- cierra 18:10 (volumen 200, incompleto: el real es 20000) y arranca 18:11.
	p.handleLiveEvent("EMAT", rawCandleEvent{Symbol: "EMAT", Timestamp: minute11, Open: f(3.19), High: f(3.19), Low: f(3.19), Close: f(3.19), Volume: f(50)})
	// 18:11 sigue en formacion, otro tick real.
	p.handleLiveEvent("EMAT", rawCandleEvent{Symbol: "EMAT", Timestamp: minute11, Open: f(3.19), High: f(3.20), Low: f(3.19), Close: f(3.20), Volume: f(90)})

	if len(closedEvents) != 1 || closedEvents[0].Timestamp != minute10 || closedEvents[0].Volume != 200 {
		t.Fatalf("expected exactly 1 closed candle so far (18:10, vol=200), got %+v", closedEvents)
	}
	formingBefore, ok := p.current["EMAT"]
	if !ok || formingBefore.Timestamp != minute11 || formingBefore.Volume != 90 {
		t.Fatalf("expected 18:11 forming with vol=90 before the late tick, got %+v (ok=%v)", formingBefore, ok)
	}

	// Correccion tardia: el trade real de 18:10 se reporto tarde, el
	// volumen final correcto es 20000. Llega DESPUES de que 18:11 ya
	// arranco -- no debe tocar la vela en formacion de 18:11.
	p.handleLiveEvent("EMAT", rawCandleEvent{Symbol: "EMAT", Timestamp: minute10, Volume: f(20000)})

	formingAfter, ok := p.current["EMAT"]
	if !ok || formingAfter.Timestamp != minute11 || formingAfter.Volume != 90 || formingAfter.Close != 3.20 {
		t.Fatalf("the late correction disturbed the REAL forming candle (18:11): %+v (ok=%v)", formingAfter, ok)
	}

	if len(closedEvents) != 2 {
		t.Fatalf("expected the late correction to be dispatched as a 2nd closed event, got %d", len(closedEvents))
	}
	corrected := closedEvents[1]
	if corrected.Timestamp != minute10 {
		t.Errorf("expected the correction to target 18:10, got %v", corrected.Timestamp)
	}
	if corrected.Volume != 20000 {
		t.Errorf("expected the corrected volume 20000, got %d", corrected.Volume)
	}
	// El resto de los campos (open/high/low/close) no vinieron en el evento
	// tardio (COMPACT: solo lo que cambio) -- deben conservarse desde
	// lastClosed, no perderse.
	if corrected.Open != 3.18 || corrected.Close != 3.18 {
		t.Errorf("expected open/close preserved from the original close (3.18), got open=%v close=%v", corrected.Open, corrected.Close)
	}

	// Un tick normal de un minuto genuinamente nuevo sigue funcionando igual.
	p.handleLiveEvent("EMAT", rawCandleEvent{Symbol: "EMAT", Timestamp: minute12, Open: f(3.21), High: f(3.21), Low: f(3.21), Close: f(3.21), Volume: f(10)})
	if len(closedEvents) != 3 || closedEvents[2].Timestamp != minute11 {
		t.Fatalf("expected 18:11 to close normally next, got %+v", closedEvents)
	}
}
