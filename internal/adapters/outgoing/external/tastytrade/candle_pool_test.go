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

// TestFlushFormingCandles_SavesBeforeDiscard confirma el fix del 2026-09-01:
// StopAllLive/CloseAllConnections descartaban la vela en formacion de cada
// simbolo sin guardarla -- confirmado en vivo que esto pasaba en CADA
// reinicio del proceso, no solo en el StopAllLive nocturno.
func TestFlushFormingCandles_SavesBeforeDiscard(t *testing.T) {
	p := newTestPool()
	ts := time.Date(2026, 9, 1, 14, 30, 0, 0, time.UTC)

	var closedEvents []domain.Candle
	p.liveSubs["AAPL"] = func(c domain.Candle) { closedEvents = append(closedEvents, c) }
	p.liveTicks["AAPL"] = func(domain.Candle) {}

	p.handleLiveEvent("AAPL", rawCandleEvent{
		Symbol: "AAPL", Timestamp: ts,
		Open: f(230), High: f(231), Low: f(229.5), Close: f(230.5), Volume: f(1500),
	})
	if _, ok := p.current["AAPL"]; !ok {
		t.Fatal("expected AAPL to be forming before the flush")
	}
	if len(closedEvents) != 0 {
		t.Fatalf("expected no closed events yet, got %+v", closedEvents)
	}

	p.flushFormingCandles()

	if len(closedEvents) != 1 {
		t.Fatalf("expected the forming candle to be dispatched as closed on flush, got %d events", len(closedEvents))
	}
	if closedEvents[0].Timestamp != ts || closedEvents[0].Volume != 1500 {
		t.Errorf("expected the flushed candle to match what was forming, got %+v", closedEvents[0])
	}
}

// TestHandleLiveEvent_RefreshReplayOfUnchangedClosedCandleDoesNotRedispatch
// reproduce lo que manda RefreshLiveSubscriptions cada minuto: reenvia el
// historial reciente de un simbolo aunque nada haya cambiado. Sin el chequeo
// de candleValuesEqual, esto reencolaba en liveSaveBuffer la MISMA vela ya
// guardada para cada uno de los ~13k simbolos en vivo, cada minuto.
func TestHandleLiveEvent_RefreshReplayOfUnchangedClosedCandleDoesNotRedispatch(t *testing.T) {
	p := newTestPool()
	minute10 := time.Date(2026, 9, 4, 18, 10, 0, 0, time.UTC)
	minute11 := time.Date(2026, 9, 4, 18, 11, 0, 0, time.UTC)

	var closedEvents []domain.Candle
	p.liveSubs["EMAT"] = func(c domain.Candle) { closedEvents = append(closedEvents, c) }
	p.liveTicks["EMAT"] = func(domain.Candle) {}

	p.handleLiveEvent("EMAT", rawCandleEvent{Symbol: "EMAT", Timestamp: minute10, Open: f(3.18), High: f(3.18), Low: f(3.18), Close: f(3.18), Volume: f(200)})
	p.handleLiveEvent("EMAT", rawCandleEvent{Symbol: "EMAT", Timestamp: minute11, Open: f(3.19), High: f(3.19), Low: f(3.19), Close: f(3.19), Volume: f(50)})
	if len(closedEvents) != 1 {
		t.Fatalf("expected 18:10 to have closed once, got %+v", closedEvents)
	}

	// Replay identico del refresco periodico: mismos valores para 18:10.
	p.handleLiveEvent("EMAT", rawCandleEvent{Symbol: "EMAT", Timestamp: minute10, Open: f(3.18), High: f(3.18), Low: f(3.18), Close: f(3.18), Volume: f(200)})
	if len(closedEvents) != 1 {
		t.Fatalf("expected the identical replay NOT to redispatch, got %d closed events", len(closedEvents))
	}

	// Una correccion real (volumen distinto) si debe seguir despachando.
	p.handleLiveEvent("EMAT", rawCandleEvent{Symbol: "EMAT", Timestamp: minute10, Volume: f(20000)})
	if len(closedEvents) != 2 || closedEvents[1].Volume != 20000 {
		t.Fatalf("expected a genuine correction to still dispatch, got %+v", closedEvents)
	}
}

// TestFlushFormingCandles_SkipsIncompleteCandle confirma que un candle sin
// OHLC completo (dispatchClosed ya filtra esto) no se guarda a medias.
func TestFlushFormingCandles_SkipsIncompleteCandle(t *testing.T) {
	p := newTestPool()
	p.current["ZZZ"] = domain.Candle{Symbol: "ZZZ", Timestamp: time.Now()}

	var closedEvents []domain.Candle
	p.liveSubs["ZZZ"] = func(c domain.Candle) { closedEvents = append(closedEvents, c) }

	p.flushFormingCandles()

	if len(closedEvents) != 0 {
		t.Fatalf("expected an incomplete candle not to be dispatched, got %+v", closedEvents)
	}
}
