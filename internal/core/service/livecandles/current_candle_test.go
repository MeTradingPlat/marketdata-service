package livecandles

import (
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestFormingPeriodStart(t *testing.T) {
	utc := time.FixedZone("UTC", 0)
	ny := time.FixedZone("NY", -4*3600)
	now := time.Date(2026, 8, 17, 18, 34, 59, 0, utc) // lunes 18:34 UTC = 14:34 ET

	cases := []struct {
		name string
		tf   domain.Timeframe
		want string
	}{
		{"M1 alinea al minuto", domain.M1, "2026-08-17T18:34:00Z"},
		{"M5 alinea a 5 minutos", domain.M5, "2026-08-17T18:30:00Z"},
		{"H1 alinea a la hora", domain.H1, "2026-08-17T18:00:00Z"},
		{"D1 alinea a medianoche UTC", domain.D1, "2026-08-17T00:00:00Z"},
		{"W1 alinea al lunes", domain.W1, "2026-08-17T00:00:00Z"},
		{"MO1 alinea al dia 1", domain.MO1, "2026-08-01T00:00:00Z"},
		{"MO3 alinea al bloque trimestral", domain.MO3, "2026-07-01T00:00:00Z"},
		{"MO6 alinea al semestre", domain.MO6, "2026-07-01T00:00:00Z"},
		{"Y1 alinea al 1 de enero", domain.Y1, "2026-01-01T00:00:00Z"},
		{"timestamp en zona ET usa el instante UTC", domain.D1, "2026-08-17T00:00:00Z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := now
			if tc.name == "timestamp en zona ET usa el instante UTC" {
				in = now.In(ny)
			}
			got := FormingPeriodStart(in, tc.tf).Format(time.RFC3339)
			if got != tc.want {
				t.Fatalf("FormingPeriodStart(%s) = %s, want %s", tc.tf, got, tc.want)
			}
		})
	}
}

func TestFormingPeriodEnd(t *testing.T) {
	// El limite se calcula desde el INICIO de periodo (lo que devuelve
	// FormingPeriodStart), no desde una fecha arbitraria.
	base := time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)
	aug1 := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	jul1 := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		start time.Time
		tf    domain.Timeframe
		want  string
	}{
		{base, domain.M1, "2026-08-17T00:01:00Z"},
		{base, domain.M5, "2026-08-17T00:05:00Z"},
		{base, domain.H1, "2026-08-17T01:00:00Z"},
		{base, domain.H4, "2026-08-17T04:00:00Z"},
		{base, domain.D1, "2026-08-18T00:00:00Z"},
		{base, domain.D2, "2026-08-19T00:00:00Z"},
		{base, domain.W1, "2026-08-24T00:00:00Z"},
		{aug1, domain.MO1, "2026-09-01T00:00:00Z"},
		{jul1, domain.MO3, "2026-10-01T00:00:00Z"},
		{jan1, domain.Y1, "2027-01-01T00:00:00Z"},
	}
	for _, tc := range cases {
		t.Run(string(tc.tf), func(t *testing.T) {
			got := FormingPeriodEnd(tc.start, tc.tf).Format(time.RFC3339)
			if got != tc.want {
				t.Fatalf("FormingPeriodEnd(%s) = %s, want %s", tc.tf, got, tc.want)
			}
		})
	}
}
