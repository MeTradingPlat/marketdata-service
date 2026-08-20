package intraday

import (
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// SnapshotTracker mantiene las sesiones intradia (pre/regular/post market)
// de todo el universo EN MEMORIA, actualizadas vela a vela conforme cierran
// en vivo -- reemplaza el GROUP BY sobre millones de filas de M1 que
// GetIntradaySessionsBatch pagaba en cada request. Confirmado en vivo el
// 2026-08-20: el chunk de hoy ya tiene 8.8M filas, y aunque el plan use el
// indice correcto, escanear por simbolo un chunk ordenado por TIEMPO (no
// por simbolo) para 8861 simbolos de una vez tarda 14s+; el fix real es no
// pagar esa consulta en el camino caliente.
type SnapshotTracker struct {
	mu   sync.RWMutex
	day  time.Time
	data map[string]domain.IntradaySnapshot
	last map[string]lastClose
}

// lastClose es el precio/volumen de la ultima M1 cerrada registrada, de
// CUALQUIER dia -- no se limpia en el corte de medianoche (a diferencia de
// data) porque el hueco entre el cierre de ayer y el primer tick de hoy
// debe seguir resolviendo al ultimo precio conocido, igual que ya hacia el
// fallback de BD que reemplaza (GetSeriesPriority M1 bars=1).
type lastClose struct {
	Price  float64
	Volume int64
}

func NewSnapshotTracker() *SnapshotTracker {
	return &SnapshotTracker{data: make(map[string]domain.IntradaySnapshot), last: make(map[string]lastClose)}
}

// Seed carga una base inicial (desde la BD, una sola vez al arrancar o tras
// el sweep M1) para que el arranque en frio no empiece en cero -- el sweep
// M1 guarda velas via CandleRepository.Save directo, sin pasar por
// RecordClosedCandle, asi que sin este seed las sesiones de hoy quedarian
// vacias hasta que llegue el primer tick en vivo de cada simbolo.
func (t *SnapshotTracker) Seed(day time.Time, snapshots map[string]domain.IntradaySnapshot) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.day = day
	t.data = snapshots
}

// SeedLastClose carga el ultimo cierre M1 conocido por simbolo (desde BD,
// una sola vez) -- mismo motivo que Seed: sin esto, LastClose no tiene nada
// que devolver hasta que cada simbolo reciba su primer tick en vivo tras el
// rollout, y GetSnapshotsBatch volveria a caer en la consulta lenta para el
// universo entero justo despues de cada despliegue.
func (t *SnapshotTracker) SeedLastClose(closes map[string]domain.Candle) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for symbol, c := range closes {
		t.last[symbol] = lastClose{Price: c.Close, Volume: c.Volume}
	}
}

// RecordClosedCandle acumula una vela M1 YA CERRADA en la sesion del dia
// que le corresponde -- mismo bucketing pre/regular/post-market que usaba
// GetIntradaySessionsBatch, pero sumando sobre todo el dia en vez de
// recalcularlo desde disco en cada request. Un cambio de dia ET limpia el
// mapa entero (nueva sesion, nadie arranca con datos de ayer).
func (t *SnapshotTracker) RecordClosedCandle(c domain.Candle) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return
	}
	tsET := c.Timestamp.In(loc)
	day := time.Date(tsET.Year(), tsET.Month(), tsET.Day(), 0, 0, 0, 0, loc)
	marketOpen := time.Date(tsET.Year(), tsET.Month(), tsET.Day(), 9, 30, 0, 0, loc)
	marketClose := time.Date(tsET.Year(), tsET.Month(), tsET.Day(), 16, 0, 0, 0, loc)

	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.day.Equal(day) {
		t.day = day
		t.data = make(map[string]domain.IntradaySnapshot)
	}

	t.last[c.Symbol] = lastClose{Price: c.Close, Volume: c.Volume}

	snap := t.data[c.Symbol]
	snap.Symbol = c.Symbol
	switch {
	case c.Timestamp.Before(marketOpen):
		snap.PreMarketVolume += c.Volume
		snap.PreMarketClose = c.Close
	case c.Timestamp.Before(marketClose):
		if snap.Open == 0 {
			snap.Open = c.Open
		}
		if snap.High == 0 || c.High > snap.High {
			snap.High = c.High
		}
		if snap.Low == 0 || c.Low < snap.Low {
			snap.Low = c.Low
		}
		snap.DayVolume += c.Volume
	default:
		snap.PostMarketVolume += c.Volume
		snap.PostMarketClose = c.Close
	}
	t.data[c.Symbol] = snap
}

// LastClose devuelve el precio/volumen de la ultima M1 cerrada registrada
// para el simbolo -- reemplaza GetSeriesPriority(..., M1, 1) como fallback
// de precio actual cuando el gateway todavia no tiene tick en vivo (recien
// suscrito, tipico justo tras un rollout: confirmado en vivo el 2026-08-20,
// esa consulta sola volvia a tardar 80s+ para el universo entero recien
// desplegado, el mismo problema de fondo que SnapshotBatch ya resolvia
// para las sesiones).
func (t *SnapshotTracker) LastClose(symbol string) (price float64, volume int64, ok bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	lc, ok := t.last[symbol]
	return lc.Price, lc.Volume, ok
}

// SnapshotBatch devuelve las sesiones ya acumuladas del lote -- un simbolo
// sin ninguna vela registrada hoy todavia (recien empezo a suscribirse en
// vivo, sin seed que lo cubriera) simplemente no aparece en el mapa; el
// caller decide si eso amerita el fallback a BD.
func (t *SnapshotTracker) SnapshotBatch(symbols []string) map[string]domain.IntradaySnapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make(map[string]domain.IntradaySnapshot, len(symbols))
	for _, sym := range symbols {
		if snap, ok := t.data[sym]; ok {
			result[sym] = snap
		}
	}
	return result
}
