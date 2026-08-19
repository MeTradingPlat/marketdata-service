package tastytrade

import (
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// Un tick llega 3 minutos despues del anterior: se emiten las 2 velas planas
// intermedias al cierre previo, con volumen 0 -- el grafico no queda con
// huecos (confirmado en vivo el 2026-08-19: AAPL sin trades en 02:12-02:13
// no dejaba vela para esos minutos).
func TestLiveGapFillFillsShortGap(t *testing.T) {
	prev := domain.Candle{Timestamp: time.Date(2026, 8, 19, 2, 11, 0, 0, time.UTC), Open: 310.1, High: 310.2, Low: 310.0, Close: 310.11, Volume: 5}
	until := time.Date(2026, 8, 19, 2, 14, 0, 0, time.UTC)

	var flats []domain.Candle
	liveGapFill(prev, until, func(c domain.Candle) { flats = append(flats, c) })

	if len(flats) != 2 {
		t.Fatalf("esperaba 2 velas planas (02:12, 02:13), obtuve %d", len(flats))
	}
	for _, f := range flats {
		if f.Open != 310.11 || f.High != 310.11 || f.Low != 310.11 || f.Close != 310.11 {
			t.Fatalf("vela plana con OHLC != cierre previo: %+v", f)
		}
		if f.Volume != 0 || f.TradeCount != 0 || f.VWAP != 0 {
			t.Fatalf("vela plana debe tener volumen/count/vwap en 0: %+v", f)
		}
	}
	if flats[0].Timestamp != until.Add(-2*time.Minute) || flats[1].Timestamp != until.Add(-time.Minute) {
		t.Fatalf("timestamps esperados 02:12 y 02:13, obtuve %v y %v", flats[0].Timestamp, flats[1].Timestamp)
	}
}

// Tick a tick continuo: sin minutos muertos, sin velas sinteticas.
func TestLiveGapFillNoGap(t *testing.T) {
	prev := domain.Candle{Timestamp: time.Date(2026, 8, 19, 2, 11, 0, 0, time.UTC), Close: 10}
	until := time.Date(2026, 8, 19, 2, 12, 0, 0, time.UTC)

	var flats []domain.Candle
	liveGapFill(prev, until, func(c domain.Candle) { flats = append(flats, c) })

	if len(flats) != 0 {
		t.Fatalf("sin hueco no debe emitir velas, obtuve %d", len(flats))
	}
}

// Hueco de 12h (quiebre de sesion real): no se inventa ninguna vela.
func TestLiveGapFillSessionBreak(t *testing.T) {
	prev := domain.Candle{Timestamp: time.Date(2026, 8, 18, 20, 0, 0, 0, time.UTC), Close: 100}
	until := time.Date(2026, 8, 19, 8, 0, 0, 0, time.UTC)

	var flats []domain.Candle
	liveGapFill(prev, until, func(c domain.Candle) { flats = append(flats, c) })

	if len(flats) != 0 {
		t.Fatalf("un quiebre de sesion no debe emitir velas planas, obtuve %d", len(flats))
	}
}

// Hueco dentro de la ventana pero al limite: 15 minutos -> 14 velas planas.
func TestLiveGapFillWindowBoundary(t *testing.T) {
	prev := domain.Candle{Timestamp: time.Date(2026, 8, 19, 2, 0, 0, 0, time.UTC), Close: 50}
	until := time.Date(2026, 8, 19, 2, 15, 0, 0, time.UTC)

	var flats []domain.Candle
	liveGapFill(prev, until, func(c domain.Candle) { flats = append(flats, c) })

	if len(flats) != 14 {
		t.Fatalf("hueco de 15 min debe emitir 14 velas (02:01..02:14), obtuve %d", len(flats))
	}
}
