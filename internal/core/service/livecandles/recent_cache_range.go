package livecandles

import (
	"sort"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

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
//
// El ULTIMO bucket solo se entrega si ya cerro de verdad (bucketEnd no
// posterior a `to`) -- antes se agregaba siempre, aunque su periodo todavia
// no hubiera terminado (ej. pedir M5 a mitad de esos 5 minutos devolvia un
// bucket a medio formar). Eso obligaba a signal-processing a descartarlo el
// mismo del lado cliente comparando contra el reloj -- confirmado en vivo
// el 2026-09-08. `to` ya trae el margen de ~1s que el caller (freshen) le
// da para dejar aterrizar el ultimo tick M1 (mismo margen que ya usa
// _BAR_CLOSE_BUFFER_SECONDS del lado de signal-processing), asi que
// reusarlo aca evita inventar un segundo mecanismo de espera.
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
	if open && !bucketEnd.After(to) {
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
