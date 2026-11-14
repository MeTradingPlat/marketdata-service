package intraday

import (
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// SeedLastClose carga el ultimo cierre M1 conocido por simbolo (desde BD,
// una sola vez) -- mismo motivo que Seed: sin esto, LastClose no tiene nada
// que devolver hasta que cada simbolo reciba su primer tick en vivo tras el
// rollout, y GetSnapshotsBatch volveria a caer en la consulta lenta para el
// universo entero justo despues de cada despliegue.
//
// requested cubre el caso de simbolos sin ninguna vela M1 en BD (warrants,
// simbolos iliquidos): se guardan con Price: 0 para que LastClose devuelva
// ok=true, evitando que GetCurrentPrices vuelva a escanear Postgres para
// buscar velas que no existen.
func (t *SnapshotTracker) SeedLastClose(requested []string, closes map[string]domain.Candle) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, sym := range requested {
		t.last[sym] = lastClose{}
	}
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

// Snapshot devuelve el snapshot intradia acumulado para un unico simbolo desde
// la memoria RAM sin consultar la base de datos.
func (t *SnapshotTracker) Snapshot(symbol string) (domain.IntradaySnapshot, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	snap, ok := t.data[symbol]
	return snap, ok
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
