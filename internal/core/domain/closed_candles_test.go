package domain

import (
	"testing"
	"time"
)

func TestClosedCandles_OlderBarsProvenByLaterBar_NoClockNeeded(t *testing.T) {
	// "now" queda a medio minuto del cierre real de bar2 (10:11) -- si el
	// filtro dependiera SOLO del reloj para bar1 y bar2, el redondeo/latencia
	// de la peticion podria excluirlas por error. Con la vela posterior en
	// el mismo lote como prueba, no hace falta el reloj para ellas.
	now := time.Date(2026, 8, 27, 18, 10, 30, 0, time.UTC)
	bar1 := Candle{Timeframe: M1, Timestamp: time.Date(2026, 8, 27, 18, 9, 0, 0, time.UTC)}
	bar2 := Candle{Timeframe: M1, Timestamp: time.Date(2026, 8, 27, 18, 10, 0, 0, time.UTC)}
	bar3Forming := Candle{Timeframe: M1, Timestamp: time.Date(2026, 8, 27, 18, 11, 0, 0, time.UTC)}

	got := ClosedCandles([]Candle{bar1, bar2, bar3Forming}, now)

	if len(got) != 2 {
		t.Fatalf("expected 2 closed candles (bar1, bar2), got %d: %+v", len(got), got)
	}
	for _, c := range got {
		if c.Timestamp.Equal(bar3Forming.Timestamp) {
			t.Errorf("bar3 (still forming, nothing after it) should have been excluded")
		}
	}
}

func TestClosedCandles_LastBarFallsBackToWallClock(t *testing.T) {
	closedBar := Candle{Timeframe: M1, Timestamp: time.Date(2026, 8, 27, 18, 9, 0, 0, time.UTC)}
	lastBar := Candle{Timeframe: M1, Timestamp: time.Date(2026, 8, 27, 18, 10, 0, 0, time.UTC)}

	// "now" ya paso el cierre real de lastBar (18:11) -- sin nada posterior
	// en el lote que la confirme, se cae al reloj y de todas formas cuenta
	// como cerrada.
	now := time.Date(2026, 8, 27, 18, 11, 30, 0, time.UTC)
	got := ClosedCandles([]Candle{closedBar, lastBar}, now)
	if len(got) != 2 {
		t.Fatalf("expected both candles closed (wall clock confirms the last one), got %d", len(got))
	}

	// "now" todavia NO llega al cierre real de lastBar (18:11) -- sin nada
	// posterior que la confirme y sin que el reloj la respalde, se descarta.
	stillForming := time.Date(2026, 8, 27, 18, 10, 30, 0, time.UTC)
	got = ClosedCandles([]Candle{closedBar, lastBar}, stillForming)
	if len(got) != 1 || !got[0].Timestamp.Equal(closedBar.Timestamp) {
		t.Fatalf("expected only closedBar, got %+v", got)
	}
}

func TestClosedCandles_EmptyInput(t *testing.T) {
	if got := ClosedCandles(nil, time.Now()); len(got) != 0 {
		t.Errorf("expected empty result for empty input, got %+v", got)
	}
}
