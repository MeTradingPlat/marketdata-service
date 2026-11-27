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
	mu      sync.RWMutex
	day     time.Time
	data    map[string]domain.IntradaySnapshot
	last    map[string]lastClose
	minutes minuteRegistry
}

// lastClose es el precio/volumen de la ultima M1 cerrada registrada, de
// CUALQUIER dia -- no se limpia en el corte de medianoche (a diferencia de
// data) porque el hueco entre el cierre de ayer y el primer tick de hoy
// debe seguir resolviendo al ultimo precio conocido, igual que ya hacia el
// fallback de BD que reemplaza (GetSeriesPriority M1 bars=1).
type lastClose struct {
	Price  float64
	Volume int64
	At     time.Time
}

func NewSnapshotTracker() *SnapshotTracker {
	return &SnapshotTracker{
		data:    make(map[string]domain.IntradaySnapshot),
		last:    make(map[string]lastClose),
		minutes: make(minuteRegistry),
	}
}

// Seed carga una base inicial (desde la BD, una sola vez al arrancar o tras
// el sweep M1) para que el arranque en frio no empiece en cero -- el sweep
// M1 guarda velas via CandleRepository.Save directo, sin pasar por
// RecordClosedCandle, asi que sin este seed las sesiones de hoy quedarian
// vacias hasta que llegue el primer tick en vivo de cada simbolo.
//
// requested es el universo completo que se le pidio a la BD: todo simbolo
// de esa lista que NO vino en snapshots (cero velas M1 hoy, no un error)
// se rellena con un IntradaySnapshot vacio para que quede PRESENTE en el
// mapa -- sin esto, GetSnapshotsBatch trata "sin filas hoy" identico a
// "todavia no se reconcilio" y le pega a Postgres con el mismo query caro
// de este mismo Seed() en CADA request de signal-processing, para siempre,
// mientras el simbolo no opere. Confirmado en vivo el 2026-09-08: fue lo
// que mantenia /fundamentals/realtime en 50-90s mucho despues de que el
// sweep de arranque ya habia terminado.
func (t *SnapshotTracker) Seed(day time.Time, requested []string, snapshots map[string]domain.IntradaySnapshot) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.day = day
	if snapshots == nil {
		snapshots = make(map[string]domain.IntradaySnapshot, len(requested))
	}
	t.data = snapshots
	t.minutes = make(minuteRegistry)
	fillMissingWithEmpty(t.data, requested)
}

// fillMissingWithEmpty agrega un IntradaySnapshot vacio para cada simbolo de
// `requested` que todavia no tiene entrada en `data` -- comparte esta logica
// Seed (reemplaza el mapa entero) y MergeReconcile (solo pisa lo que vino en
// esta vuelta) porque ambos necesitan la misma garantia de cobertura sin
// pisar jamas una entrada que ya existe (real o un vacio de una vuelta
// anterior).
func fillMissingWithEmpty(data map[string]domain.IntradaySnapshot, requested []string) {
	for _, symbol := range requested {
		if _, ok := data[symbol]; !ok {
			data[symbol] = domain.IntradaySnapshot{Symbol: symbol}
		}
	}
}

// MergeReconcile corrige entradas puntuales contra la BD (fuente de verdad)
// SIN el reset completo de Seed -- pensado para llamarse repetidas veces
// durante el dia (ver StartSnapshotReseedLoop), a diferencia de Seed que
// solo es seguro una vez (al arrancar o en la ventana de mantenimiento).
// La consulta de lote que alimenta esto puede volver INCOMPLETA bajo carga
// de Postgres (mismo caso ya documentado en seedSnapshotTracker) -- un
// Seed() con ese resultado parcial borraria de un tiron el tracker entero
// menos esos pocos simbolos. Aca en cambio solo se pisan los simbolos que
// SI vinieron en snapshots; el resto conserva lo que ya tenia acumulado via
// RecordClosedCandle en vez de perderlo. Un dia distinto SI dispara el
// mismo reset de Seed (nadie deberia arrancar el dia con datos de ayer).
//
// requested cubre el mismo caso que en Seed (ver su comentario): un simbolo
// pedido que sigue sin ninguna entrada tras esta vuelta (nunca opero hoy,
// ni en esta consulta ni en ninguna anterior) se rellena con un
// IntradaySnapshot vacio. Solo se rellena si NO habia entrada previa -- un
// simbolo que ya tiene datos (reales o un vacio de una vuelta anterior) se
// deja intacto, que es exactamente lo que evita que una vuelta incompleta
// por presion de Postgres borre volumen real ya acumulado.
func (t *SnapshotTracker) MergeReconcile(day time.Time, requested []string, snapshots map[string]domain.IntradaySnapshot) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.day.Equal(day) {
		t.day = day
		t.data = make(map[string]domain.IntradaySnapshot)
		t.minutes = make(minuteRegistry)
	}
	for symbol, snap := range snapshots {
		t.data[symbol] = snap
		delete(t.minutes, symbol)
	}
	fillMissingWithEmpty(t.data, requested)
}
