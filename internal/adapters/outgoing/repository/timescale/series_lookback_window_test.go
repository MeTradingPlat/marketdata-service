package timescale

import (
	"testing"
	"time"
)

func TestSeriesLookbackWindow(t *testing.T) {
	day := 24 * time.Hour
	cases := []struct {
		name     string
		bars     int
		duration time.Duration
		wantMin  time.Duration
		wantMax  time.Duration
	}{
		// D1 con bars grande (caso real: pivots pide 4 anios = 1038 barras).
		// Antes del fix, bars*3+60 daba ~8.7 anios; ahora debe rondar ~4.7.
		{"D1 pivots bars grande", 1038, day, 4*365*day + 200*day, 5*365*day},
		// D1 con bars chico (caso real: prueba daniel, RANGE_EXTREME_PROXIMITY
		// con LOOKBACK_VELAS=20+1) -- no debe haber regresion frente a la
		// formula generica vieja (~123 dias).
		{"D1 bars chico", 21, day, 90 * day, 130 * day},
		// M1 con bars=1 (SeedLastClose) -- debe seguir cayendo en minLookback
		// (10 dias), no en la formula nueva de D1+.
		{"M1 bars minimo cae en minLookback", 1, time.Minute, 10 * day, 10 * day},
		// Intradia con bars grande no debe activar la rama D1+.
		{"M5 bars grande sigue formula generica", 1000, 5 * time.Minute, 3000 * 5 * time.Minute, 3060 * 5 * time.Minute},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := seriesLookbackWindow(c.bars, c.duration)
			if got < c.wantMin || got > c.wantMax {
				t.Errorf("seriesLookbackWindow(%d, %v) = %v, want between %v and %v",
					c.bars, c.duration, got, c.wantMin, c.wantMax)
			}
		})
	}
}
