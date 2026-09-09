package tastytrade

import (
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// TestCloseElapsedForming_ClosesCandleWhosePeriodHasElapsed confirma el fix
// del cierre por reloj: un simbolo poco liquido que no recibe ningun tick
// del minuto siguiente ya no se queda "en formacion" para siempre -- el
// sweep periodico lo cierra solo con que haya pasado el minuto + el margen
// de gracia.
func TestCloseElapsedForming_ClosesCandleWhosePeriodHasElapsed(t *testing.T) {
	p := newTestPool()
	minute10 := time.Date(2026, 9, 9, 18, 10, 0, 0, time.UTC)

	var closedEvents []domain.Candle
	p.liveSubs["QUIET"] = func(c domain.Candle) { closedEvents = append(closedEvents, c) }
	p.liveTicks["QUIET"] = func(domain.Candle) {}

	p.handleLiveEvent("QUIET", rawCandleEvent{Symbol: "QUIET", Timestamp: minute10, Open: f(1.0), High: f(1.0), Low: f(1.0), Close: f(1.0), Volume: f(10)})

	now := minute10.Add(time.Minute + closeElapsedGracePeriod)
	p.CloseElapsedForming(now)

	if _, stillForming := p.current["QUIET"]; stillForming {
		t.Fatal("expected the elapsed candle to be removed from current")
	}
	if len(closedEvents) != 1 || closedEvents[0].Timestamp != minute10 || closedEvents[0].Volume != 10 {
		t.Fatalf("expected the elapsed candle to be dispatched as closed, got %+v", closedEvents)
	}
	last, ok := p.lastClosed["QUIET"]
	if !ok || last.Timestamp != minute10 {
		t.Fatalf("expected lastClosed to record the force-closed candle, got %+v (ok=%v)", last, ok)
	}
}

// TestCloseElapsedForming_LeavesFreshCandleUntouched confirma que el sweep
// no cierra de mas: una vela cuyo minuto AUN no termino (o esta dentro del
// margen de gracia) debe seguir en formacion.
func TestCloseElapsedForming_LeavesFreshCandleUntouched(t *testing.T) {
	p := newTestPool()
	minute10 := time.Date(2026, 9, 9, 18, 10, 0, 0, time.UTC)

	var closedEvents []domain.Candle
	p.liveSubs["FRESH"] = func(c domain.Candle) { closedEvents = append(closedEvents, c) }
	p.liveTicks["FRESH"] = func(domain.Candle) {}

	p.handleLiveEvent("FRESH", rawCandleEvent{Symbol: "FRESH", Timestamp: minute10, Open: f(1.0), High: f(1.0), Low: f(1.0), Close: f(1.0), Volume: f(10)})

	// Todavia dentro del minuto + el margen de gracia -- no debe cerrar.
	p.CloseElapsedForming(minute10.Add(30 * time.Second))

	if _, stillForming := p.current["FRESH"]; !stillForming {
		t.Fatal("expected the fresh candle to remain forming")
	}
	if len(closedEvents) != 0 {
		t.Fatalf("expected no closed events yet, got %+v", closedEvents)
	}
}

// TestHandleLiveEvent_LateTickAfterForceCloseMergesAsCorrection confirma que
// un tick que llega justo despues de que CloseElapsedForming ya cerro por
// tiempo esa vela se trata como correccion tardia (fusiona contra
// lastClosed), no como el arranque de una vela "nueva" que reabriria un
// minuto que el sistema ya dio por cerrado y publico.
func TestHandleLiveEvent_LateTickAfterForceCloseMergesAsCorrection(t *testing.T) {
	p := newTestPool()
	minute10 := time.Date(2026, 9, 9, 18, 10, 0, 0, time.UTC)

	var closedEvents []domain.Candle
	p.liveSubs["QUIET"] = func(c domain.Candle) { closedEvents = append(closedEvents, c) }
	p.liveTicks["QUIET"] = func(domain.Candle) {}

	p.handleLiveEvent("QUIET", rawCandleEvent{Symbol: "QUIET", Timestamp: minute10, Open: f(1.0), High: f(1.0), Low: f(1.0), Close: f(1.0), Volume: f(10)})
	p.CloseElapsedForming(minute10.Add(time.Minute + closeElapsedGracePeriod))
	if len(closedEvents) != 1 {
		t.Fatalf("expected the force-close to dispatch once, got %+v", closedEvents)
	}

	// El trade real de ese mismo minuto llega tarde, con mas volumen del que
	// alcanzo a ver el sweep.
	p.handleLiveEvent("QUIET", rawCandleEvent{Symbol: "QUIET", Timestamp: minute10, Volume: f(9999)})

	if _, reopened := p.current["QUIET"]; reopened {
		t.Fatal("the late tick must not reopen minute10 as a new forming candle")
	}
	if len(closedEvents) != 2 {
		t.Fatalf("expected the late tick to dispatch as a correction, got %+v", closedEvents)
	}
	corrected := closedEvents[1]
	if corrected.Timestamp != minute10 || corrected.Volume != 9999 {
		t.Errorf("expected the correction to target minute10 with vol=9999, got %+v", corrected)
	}
	if corrected.Open != 1.0 || corrected.Close != 1.0 {
		t.Errorf("expected open/close preserved from the force-closed candle, got open=%v close=%v", corrected.Open, corrected.Close)
	}

	// Un tick de un minuto genuinamente nuevo despues de la correccion sigue
	// arrancando una vela nueva con normalidad.
	minute11 := minute10.Add(time.Minute)
	p.handleLiveEvent("QUIET", rawCandleEvent{Symbol: "QUIET", Timestamp: minute11, Open: f(1.1), High: f(1.1), Low: f(1.1), Close: f(1.1), Volume: f(5)})
	forming, ok := p.current["QUIET"]
	if !ok || forming.Timestamp != minute11 {
		t.Fatalf("expected minute11 to start forming normally, got %+v (ok=%v)", forming, ok)
	}
}
