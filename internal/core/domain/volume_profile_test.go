package domain

import (
	"testing"
	"time"
)

func newYork(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("sin tzdata: %v", err)
	}
	return loc
}

func TestSlotOf_AperturaDelPremercadoEsElSlotCeroEnVeranoEInvierno(t *testing.T) {
	loc := newYork(t)
	cases := map[string]time.Time{
		"verano (EDT)":   time.Date(2026, 9, 18, 8, 0, 0, 0, time.UTC),
		"invierno (EST)": time.Date(2026, 12, 15, 9, 0, 0, 0, time.UTC),
	}
	for name, ts := range cases {
		session, slot, ok := SlotOf(ts, loc)
		if !ok || slot != 0 {
			t.Fatalf("%s: slot=%d ok=%v, want 0/true", name, slot, ok)
		}
		if session == 0 {
			t.Fatalf("%s: session vacia", name)
		}
	}
}

func TestSlotOf_ElPostmercadoTardioSigueEnLaMismaSesionEnInvierno(t *testing.T) {
	loc := newYork(t)
	antes, _, _ := SlotOf(time.Date(2026, 12, 15, 23, 30, 0, 0, time.UTC), loc)
	despues, slot, ok := SlotOf(time.Date(2026, 12, 16, 0, 59, 0, 0, time.UTC), loc)

	if !ok || slot != VolumeProfileSlots-1 || antes != despues {
		t.Fatalf("las 19:59 ET deben ser el ultimo slot de la misma sesion: ok=%v slot=%d %d/%d", ok, slot, antes, despues)
	}
}

func TestSlotOf_FueraDeHorarioNoTieneSlot(t *testing.T) {
	loc := newYork(t)
	if _, _, ok := SlotOf(time.Date(2026, 9, 18, 7, 59, 0, 0, time.UTC), loc); ok {
		t.Fatal("03:59 ET no pertenece a la sesion extendida")
	}
	if _, _, ok := SlotOf(time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC), loc); ok {
		t.Fatal("20:00 ET ya es fuera de la sesion extendida")
	}
}

func TestSessionVolumes_PromediaElAcumuladoDeLasSesiones(t *testing.T) {
	v := NewSessionVolumes()
	v.Add(20260917, 0, 100)
	v.Add(20260917, 2, 50)
	v.Add(20260918, 0, 300)

	sessions, cum := v.Profile(20)

	if sessions != 2 {
		t.Fatalf("sessions = %d, want 2", sessions)
	}
	if cum[0] != 200 || cum[1] != 200 || cum[2] != 225 || cum[VolumeProfileSlots-1] != 225 {
		t.Fatalf("acumulado promedio inesperado: %v %v %v %v", cum[0], cum[1], cum[2], cum[VolumeProfileSlots-1])
	}
}

func TestSessionVolumes_SoloUsaLasSesionesMasRecientes(t *testing.T) {
	v := NewSessionVolumes()
	v.Add(20260910, 0, 1000)
	v.Add(20260917, 0, 10)
	v.Add(20260918, 0, 30)

	sessions, cum := v.Profile(2)

	if sessions != 2 || cum[0] != 20 {
		t.Fatalf("sessions=%d cum[0]=%v, want 2 y 20", sessions, cum[0])
	}
}

func TestSessionVolumes_SinDatosNoDevuelvePerfil(t *testing.T) {
	sessions, cum := NewSessionVolumes().Profile(20)

	if sessions != 0 || cum != nil {
		t.Fatalf("sessions=%d cum=%v, want 0/nil", sessions, cum)
	}
}

func TestSlotMinutes_SoloAceptaTimeframesQueDividenLaSesion(t *testing.T) {
	if m, ok := SlotMinutes(M1); !ok || m != 1 {
		t.Fatalf("M1 = %d/%v", m, ok)
	}
	if m, ok := SlotMinutes(M5); !ok || m != 5 {
		t.Fatalf("M5 = %d/%v", m, ok)
	}
	if _, ok := SlotMinutes(D1); ok {
		t.Fatal("D1 no tiene perfil por franja")
	}
}

func TestDownsampleCumulative_TomaElAcumuladoAlCierreDeCadaFranja(t *testing.T) {
	cum := make([]float32, VolumeProfileSlots)
	for i := range cum {
		cum[i] = float32(i + 1)
	}

	got := DownsampleCumulative(cum, 5)

	if len(got) != VolumeProfileSlots/5 || got[0] != 5 || got[1] != 10 {
		t.Fatalf("len=%d first=%v", len(got), got[:2])
	}
}
