package livecandles

import (
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// RecentCache guarda las ultimas `maxBars` velas M1 por simbolo, indexadas
// por su propio timestamp -- a diferencia de Postgres, leer de aca nunca
// depende de si una escritura ya es visible: el mismo proceso que la
// guarda es el que la sirve. Una correccion tardia (un tick que llega para
// un minuto ya pasado) actualiza DIRECTAMENTE la entrada de ese minuto por
// su propia llave, sin tocar ni cerrar ninguna otra -- confirmado en vivo
// el 2026-08-27 con EMAT: el volumen real de un pico de un minuto tardo
// varios ciclos del escaner en reflejarse completo rio abajo.
//
// Por barras, no por minutos: un simbolo ralo (minutos sin ningun trade,
// confirmado en vivo con EMAT: 18:07 y 18:12 sin vela real) guarda menos
// barras utiles en una ventana de tiempo fija que uno liquido -- por
// cantidad de barras coincide con como los escaneres ya piden su historia
// (bars_necesarias_grupo), sin importar cuanto tiempo real representa.
type RecentCache struct {
	mu      sync.RWMutex
	maxBars int
	ttl     time.Duration
	data    map[string]map[int64]cachedCandle
}

type cachedCandle struct {
	candle domain.Candle
	closed bool
}

// DefaultRecentCacheBars: margen sobre las ~15 barras M1 que la mayoria de
// los filtros tecnicos piden de historia reciente.
const DefaultRecentCacheBars = 20

// DefaultRecentCacheTTL: red de seguridad para un simbolo que dejo de
// recibir ticks del todo (desuscrito, o sin ningun trade en mucho tiempo)
// -- sin esto, sus barras viejas se quedarian en memoria para siempre
// porque el recorte por cantidad solo se dispara al llegar una vela nueva.
const DefaultRecentCacheTTL = 15 * time.Minute

func NewRecentCache(maxBars int, ttl time.Duration) *RecentCache {
	return &RecentCache{maxBars: maxBars, ttl: ttl, data: make(map[string]map[int64]cachedCandle)}
}

// NewDefaultRecentCache es el constructor que usa el contenedor de DI --
// dig no puede inferir un int/time.Duration crudo sin ambiguedad, asi que
// la version parametrizada de arriba queda para tests.
func NewDefaultRecentCache() *RecentCache {
	return NewRecentCache(DefaultRecentCacheBars, DefaultRecentCacheTTL)
}

// Put guarda o reemplaza la vela por su PROPIO timestamp -- upsert por
// llave, nunca por orden de llegada, asi una correccion tardia para un
// minuto viejo no puede confundirse con una vela nueva ni pisar la que
// esta en formacion. Si el simbolo supera maxBars, descarta la mas vieja.
func (c *RecentCache) Put(candle domain.Candle, closed bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	bucket := c.data[candle.Symbol]
	if bucket == nil {
		bucket = make(map[int64]cachedCandle)
		c.data[candle.Symbol] = bucket
	}
	bucket[candle.Timestamp.Unix()] = cachedCandle{candle: candle, closed: closed}
	for len(bucket) > c.maxBars {
		delete(bucket, oldestKey(bucket))
	}
}

// Get devuelve la vela cacheada de un timestamp exacto -- el caller lo usa
// como base para fusionar una correccion tardia (dxLink en formato COMPACT
// solo reenvia los campos que cambiaron; sin la base se pierden los que no).
func (c *RecentCache) Get(symbol string, ts time.Time) (domain.Candle, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.data[symbol][ts.Unix()]
	return entry.candle, ok
}

func oldestKey(bucket map[int64]cachedCandle) int64 {
	var oldest int64
	first := true
	for ts := range bucket {
		if first || ts < oldest {
			oldest = ts
			first = false
		}
	}
	return oldest
}
