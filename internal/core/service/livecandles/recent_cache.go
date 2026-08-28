package livecandles

import (
	"sort"
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// RecentCache guarda las velas M1 de los ultimos `ttl` minutos por simbolo,
// indexadas por su propio timestamp -- a diferencia de Postgres, leer de aca
// nunca depende de si una escritura ya es visible: el mismo proceso que la
// guarda es el que la sirve. Una correccion tardia (un tick que llega para
// un minuto ya pasado) actualiza DIRECTAMENTE la entrada de ese minuto por
// su propia llave, sin tocar ni cerrar ninguna otra -- confirmado en vivo
// el 2026-08-27 con EMAT: el volumen real de un pico de un minuto tardo
// varios ciclos del escaner en reflejarse completo rio abajo.
type RecentCache struct {
	mu   sync.RWMutex
	ttl  time.Duration
	data map[string]map[int64]cachedCandle
}

type cachedCandle struct {
	candle domain.Candle
	closed bool
}

// DefaultRecentCacheTTL: 15 minutos alcanza de sobra cualquier vela que
// pueda seguir recibiendo correcciones tardias, sin acumular memoria por
// todo el universo de simbolos indefinidamente.
const DefaultRecentCacheTTL = 15 * time.Minute

func NewRecentCache(ttl time.Duration) *RecentCache {
	return &RecentCache{ttl: ttl, data: make(map[string]map[int64]cachedCandle)}
}

// NewDefaultRecentCache es el constructor que usa el contenedor de DI --
// dig no puede inferir un time.Duration crudo sin ambiguedad, asi que la
// version parametrizada de arriba queda para tests.
func NewDefaultRecentCache() *RecentCache {
	return NewRecentCache(DefaultRecentCacheTTL)
}

// Put guarda o reemplaza la vela por su PROPIO timestamp -- upsert por
// llave, nunca por orden de llegada, asi una correccion tardia para un
// minuto viejo no puede confundirse con una vela nueva ni pisar la que
// esta en formacion.
func (c *RecentCache) Put(candle domain.Candle, closed bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	bucket := c.data[candle.Symbol]
	if bucket == nil {
		bucket = make(map[int64]cachedCandle)
		c.data[candle.Symbol] = bucket
	}
	bucket[candle.Timestamp.Unix()] = cachedCandle{candle: candle, closed: closed}
}

// Range devuelve las velas cacheadas del simbolo en [from, to), ordenadas
// por tiempo -- el caller decide que hacer con lo que ya salio de la
// ventana de retencion (tipicamente, ir a Postgres por eso).
func (c *RecentCache) Range(symbol string, from, to time.Time) []domain.Candle {
	c.mu.RLock()
	defer c.mu.RUnlock()
	bucket := c.data[symbol]
	if len(bucket) == 0 {
		return nil
	}
	result := make([]domain.Candle, 0, len(bucket))
	for ts, entry := range bucket {
		t := time.Unix(ts, 0).UTC()
		if !t.Before(from) && t.Before(to) {
			result = append(result, entry.candle)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Timestamp.Before(result[j].Timestamp) })
	return result
}

// OldestCovered devuelve el timestamp mas viejo cacheado del simbolo -- el
// caller lo usa para saber desde donde ya no necesita ir a Postgres.
func (c *RecentCache) OldestCovered(symbol string) (time.Time, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	bucket := c.data[symbol]
	if len(bucket) == 0 {
		return time.Time{}, false
	}
	var oldest int64
	first := true
	for ts := range bucket {
		if first || ts < oldest {
			oldest = ts
			first = false
		}
	}
	return time.Unix(oldest, 0).UTC(), true
}

// Evict descarta las entradas mas viejas que ttl -- llamado periodicamente
// (ver cmd/api), no en el camino caliente de cada tick.
func (c *RecentCache) Evict(now time.Time) {
	cutoff := now.Add(-c.ttl).Unix()
	c.mu.Lock()
	defer c.mu.Unlock()
	for symbol, bucket := range c.data {
		for ts := range bucket {
			if ts < cutoff {
				delete(bucket, ts)
			}
		}
		if len(bucket) == 0 {
			delete(c.data, symbol)
		}
	}
}
