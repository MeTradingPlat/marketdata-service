package intraday

import (
	"sync"
	"time"
)

// DayVolumeTracker guarda el volumen real del dia por simbolo, resuelto en
// vivo via el evento Trade de DxLink (Trade.dayVolume) -- a diferencia del
// DayVolume que ya mantiene SnapshotTracker (suma de Candle.volume, que solo
// trae 40-60% del consolidado real, confirmado en vivo el 2026-09-22), este
// es el numero que de verdad coincide con lo que reporta TastyTrade. Se
// refresca por lotes en segundo plano (ver StartDayVolumeRefreshLoop), no
// vela a vela -- Trade.dayVolume ya llega acumulado desde dxFeed, no hace
// falta reconstruirlo tick a tick.
type DayVolumeTracker struct {
	mu           sync.RWMutex
	day          time.Time
	volumes      map[string]int64
	preMarketEnd map[string]int64
	regularEnd   map[string]int64
}

func NewDayVolumeTracker() *DayVolumeTracker {
	return &DayVolumeTracker{
		volumes:      make(map[string]int64),
		preMarketEnd: make(map[string]int64),
		regularEnd:   make(map[string]int64),
	}
}

// Update reemplaza el volumen de cada simbolo del lote -- un simbolo que
// esta ronda no trajo dato (illiquido, sin trades hoy) conserva lo que ya
// tenia, igual que SnapshotTracker.MergeReconcile: una vuelta incompleta
// por presion de DxLink no debe borrar un volumen real ya conocido.
func (t *DayVolumeTracker) Update(day time.Time, volumes map[string]int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.resetForDayLocked(day)
	for symbol, volume := range volumes {
		t.volumes[symbol] = volume
	}
}

// Get devuelve el volumen real si ya se resolvio para el simbolo -- el
// llamador decide el fallback (SnapshotTracker.DayVolume) cuando no.
func (t *DayVolumeTracker) Get(symbol string) (int64, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	v, ok := t.volumes[symbol]
	return v, ok
}
