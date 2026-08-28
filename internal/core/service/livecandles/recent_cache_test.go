package livecandles

import (
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestRecentCache_LateCorrectionUpdatesOwnKeyOnly(t *testing.T) {
	c := NewRecentCache(20, 15*time.Minute)
	base := time.Date(2026, 8, 27, 18, 10, 0, 0, time.UTC)

	c.Put(domain.Candle{Symbol: "EMAT", Timestamp: base.Add(14 * time.Minute), Volume: 20000}, true)
	c.Put(domain.Candle{Symbol: "EMAT", Timestamp: base.Add(15 * time.Minute), Volume: 100}, false)

	// Correccion tardia del minuto 14 (llega despues de que el 15 ya esta en
	// formacion) -- no debe tocar el 15.
	c.Put(domain.Candle{Symbol: "EMAT", Timestamp: base.Add(14 * time.Minute), Volume: 27165}, true)

	got := c.Range("EMAT", base.Add(14*time.Minute), base.Add(16*time.Minute))
	if len(got) != 2 {
		t.Fatalf("expected 2 candles, got %d", len(got))
	}
	if got[0].Volume != 27165 {
		t.Errorf("minute 14 should reflect the corrected volume, got %d", got[0].Volume)
	}
	if got[1].Volume != 100 {
		t.Errorf("minute 15 should be untouched by the correction, got %d", got[1].Volume)
	}
}

func TestRecentCache_EvictDropsOlderThanTTL(t *testing.T) {
	c := NewRecentCache(20, 5*time.Minute)
	now := time.Date(2026, 8, 27, 18, 20, 0, 0, time.UTC)

	c.Put(domain.Candle{Symbol: "EMAT", Timestamp: now.Add(-10 * time.Minute), Volume: 1}, true)
	c.Put(domain.Candle{Symbol: "EMAT", Timestamp: now.Add(-1 * time.Minute), Volume: 2}, true)

	c.Evict(now)

	got := c.Range("EMAT", now.Add(-time.Hour), now.Add(time.Minute))
	if len(got) != 1 {
		t.Fatalf("expected 1 candle after evict, got %d", len(got))
	}
	if got[0].Volume != 2 {
		t.Errorf("the surviving candle should be the recent one, got volume %d", got[0].Volume)
	}
}

func TestRecentCache_TrimsToMaxBarsRegardlessOfElapsedTime(t *testing.T) {
	// Por cantidad de barras, no por minutos: EMAT tuvo minutos sin ningun
	// trade real (18:07, 18:12) -- una ventana de tiempo fija guardaria
	// menos barras utiles que una liquida. Aca simulamos eso: 5 velas con
	// huecos reales en el medio, maxBars=3 debe quedarse con las 3 mas
	// recientes sin importar que representan mas de 3 minutos de reloj.
	c := NewRecentCache(3, 15*time.Minute)
	base := time.Date(2026, 8, 27, 18, 5, 0, 0, time.UTC)

	c.Put(domain.Candle{Symbol: "EMAT", Timestamp: base, Volume: 1}, true)
	c.Put(domain.Candle{Symbol: "EMAT", Timestamp: base.Add(1 * time.Minute), Volume: 2}, true)
	c.Put(domain.Candle{Symbol: "EMAT", Timestamp: base.Add(3 * time.Minute), Volume: 3}, true) // hueco: sin 18:07
	c.Put(domain.Candle{Symbol: "EMAT", Timestamp: base.Add(4 * time.Minute), Volume: 4}, true)
	c.Put(domain.Candle{Symbol: "EMAT", Timestamp: base.Add(8 * time.Minute), Volume: 5}, true) // hueco: sin 18:12

	got := c.Range("EMAT", base.Add(-time.Hour), base.Add(time.Hour))
	if len(got) != 3 {
		t.Fatalf("expected 3 candles (maxBars), got %d", len(got))
	}
	for i, want := range []int64{3, 4, 5} {
		if got[i].Volume != want {
			t.Errorf("position %d: expected volume %d, got %d", i, want, got[i].Volume)
		}
	}
}

func TestRecentCache_OldestCovered(t *testing.T) {
	c := NewRecentCache(20, 15*time.Minute)
	base := time.Date(2026, 8, 27, 18, 10, 0, 0, time.UTC)

	if _, ok := c.OldestCovered("EMAT"); ok {
		t.Fatal("expected no coverage for an empty cache")
	}

	c.Put(domain.Candle{Symbol: "EMAT", Timestamp: base.Add(2 * time.Minute)}, true)
	c.Put(domain.Candle{Symbol: "EMAT", Timestamp: base}, true)

	oldest, ok := c.OldestCovered("EMAT")
	if !ok || !oldest.Equal(base) {
		t.Errorf("expected oldest=%v, got %v (ok=%v)", base, oldest, ok)
	}
}
