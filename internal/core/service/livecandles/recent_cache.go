package livecandles

import (
	"sort"
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

// Range devuelve las velas cacheadas del simbolo en [from, to), ordenadas
// por tiempo -- el caller decide que hacer con lo que ya salio de la
// ventana cubierta (tipicamente, ir a Postgres por eso).
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

// RangeAggregated pliega las velas M1 cacheadas del simbolo en buckets de
// `bucket` de ancho -- el mismo plegado que ya usa GetCurrentCandle para la
// vela en formacion de un timeframe derivado (foldM1Bar/foldBar), aplicado
// aca tambien a los buckets ya cerrados dentro de la ventana que cubre el
// cache. Alineado por duracion exacta desde la epoca UTC (Truncate), misma
// convencion que la agregacion SQL de Postgres (time_bucket) y que
// FormingPeriodStart -- no hace falta un cache nuevo por timeframe, M1 ya
// tiene todo lo necesario para armar cualquier derivado al vuelo.
func (c *RecentCache) RangeAggregated(symbol string, from, to time.Time, bucket time.Duration, timeframe domain.Timeframe) []domain.Candle {
	m1 := c.Range(symbol, from, to)
	if len(m1) == 0 {
		return nil
	}
	result := make([]domain.Candle, 0, len(m1))
	var current domain.Candle
	var bucketEnd time.Time
	open := false
	for _, bar := range m1 {
		if !open || !bar.Timestamp.Before(bucketEnd) {
			if open {
				result = append(result, current)
			}
			bucketStart := bar.Timestamp.Truncate(bucket)
			bucketEnd = bucketStart.Add(bucket)
			current = domain.Candle{
				Symbol: symbol, Timeframe: timeframe, Timestamp: bucketStart,
				Open: bar.Open, High: bar.High, Low: bar.Low, Close: bar.Close,
				Volume: bar.Volume, Source: bar.Source,
			}
			open = true
			continue
		}
		if bar.High > current.High {
			current.High = bar.High
		}
		if bar.Low < current.Low {
			current.Low = bar.Low
		}
		current.Close = bar.Close
		current.Volume += bar.Volume
	}
	if open {
		result = append(result, current)
	}
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
	return time.Unix(oldestKey(bucket), 0).UTC(), true
}

// Evict descarta las entradas mas viejas que ttl -- red de seguridad para
// simbolos que dejaron de recibir ticks del todo (ver comentario de
// DefaultRecentCacheTTL). Llamado periodicamente (ver cmd/api), no en el
// camino caliente de cada tick.
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
