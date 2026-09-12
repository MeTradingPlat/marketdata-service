package ingestion

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
	"golang.org/x/sync/singleflight"
)

// candleCacheTTL: las consultas de velas repetidas (el frontend cambia de
// timeframe ida y vuelta, signal-processing repite el batch del universo)
// volvian a pagar la agregacion derivada sobre M1 comprimida cada vez --
// confirmado en vivo: ~10 queries time_bucket concurrentes de 30-90s cada
// una, con 6-7k ExclusiveLocks por query sobre chunks comprimidos que
// ahogaban el lock manager de postgres y dejaban TODO el servicio lento.
// Cachear el resultado 60s (las velas cerradas no cambian; a lo sumo falta
// la vela en formacion, que el WS sirve por separado) colapsa el patron de
// repeticion sin tocar la frescura visible.
const candleCacheTTL = 60 * time.Second

// candleCacheMaxEntries: 20000 (valor original) podia pesar hasta ~400MB
// en el peor caso (bars=151 tipico, ~19KB por entrada) -- confirmado en
// vivo el 2026-08-23 que el contenedor ya usa 773MB de un limite duro de
// 1GB (--memory 1g --memory-swap 1g en cd.yml, sin colchon de swap), asi
// que llenar el cache entero lo hubiera empujado a que Docker lo matara
// por OOM. 8000 entradas topea el peor caso en ~150MB, dejando margen real
// bajo el limite actual sin tocar la asignacion de memoria del contenedor
// (el VAIO entero ya esta ajustado: 915MB libres compartidos entre 26
// contenedores, subir el limite del contenedor le resta margen a los demas).
const candleCacheMaxEntries = 8000

type candleCacheEntry struct {
	expires time.Time
	candles []domain.Candle
}

type candleCache struct {
	mu      sync.Mutex
	entries map[string]candleCacheEntry
}

func (c *candleCache) get(key string) ([]domain.Candle, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expires) {
		delete(c.entries, key)
		return nil, false
	}
	return entry.candles, true
}

func (c *candleCache) put(key string, candles []domain.Candle) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= candleCacheMaxEntries {
		c.evictOneLocked()
	}
	c.entries[key] = candleCacheEntry{expires: time.Now().Add(candleCacheTTL), candles: candles}
}

// evictOneLocked libera espacio para la entrada nueva -- primero intenta
// una ya vencida (barata de perder, iba a expirar sola de todas formas);
// solo si no encuentra ninguna en la primera vuelta de iteracion cae a
// borrar la primera que encuentre. El orden de iteracion de un map en Go es
// aleatorio, asi que sin este intento previo un TTL corto (60s) bajo carga
// alta (candleCacheMaxEntries=20000 lleno) podia desalojar una entrada
// recien puesta en vez de una que ya no serve a nadie.
func (c *candleCache) evictOneLocked() {
	now := time.Now()
	for k, entry := range c.entries {
		if now.After(entry.expires) {
			delete(c.entries, k)
			return
		}
	}
	for k := range c.entries {
		delete(c.entries, k)
		return
	}
}

type getCandlesService struct {
	repo        out.CandleRepository
	cache       candleCache
	recentCache *livecandles.RecentCache
	// fetchGroup colapsa pedidos concurrentes de la MISMA clave (simbolo +
	// timeframe + bars + before) en una sola consulta real -- sin esto, dos
	// llamadas que llegan al mismo tiempo (ej. dos pestañas del frontend
	// mirando el mismo simbolo, o un escaner y un pivots pidiendo D1 de AAPL
	// a la vez) ambas ven el cache vacio y pagan la misma consulta a
	// Postgres por duplicado -- mismo patron ya usado en oauth.go para el
	// mismo tipo de problema (confirmado en vivo alla: llamadas concurrentes
	// duplicando trabajo real).
	fetchGroup singleflight.Group
	// coalescer: mismo motivo que fetchGroup, para GetCandlesBatch -- ver
	// batchCoalescer.
	coalescer *batchCoalescer
}

func NewGetCandlesService(repo out.CandleRepository, recentCache *livecandles.RecentCache) in.GetCandlesService {
	return &getCandlesService{
		repo: repo, cache: candleCache{entries: make(map[string]candleCacheEntry)},
		recentCache: recentCache, coalescer: newBatchCoalescer(),
	}
}

func (s *getCandlesService) GetCandles(ctx context.Context, symbol string, timeframe domain.Timeframe, bars int, before *time.Time) ([]domain.Candle, error) {
	key := candleCacheKey(symbol, timeframe, bars, before)
	var candles []domain.Candle
	if cached, ok := s.cache.get(key); ok {
		candles = cached
	} else {
		fetched, err, _ := s.fetchGroup.Do(key, func() (interface{}, error) {
			return s.repo.GetCandles(ctx, symbol, timeframe, bars, before)
		})
		if err != nil {
			return nil, fmt.Errorf("getting candles for %s %s: %w", symbol, timeframe, err)
		}
		candles = fetched.([]domain.Candle)
		s.cache.put(key, candles)
	}
	// "hasta ahora" (before=nil): la cola de lo que RecentCache ya cubre se
	// reemplaza por su version agregada en vivo, sin importar si el resto
	// vino del cache de 60s o de Postgres recien -- ninguno de los dos
	// garantiza que la fila mas reciente ya sea visible/completa (confirmado
	// en vivo el 2026-08-27 con EMAT: volumen incompleto de una vela recien
	// cerrada). Una fecha puntual (before != nil) es historico ya cerrado,
	// no necesita nada de esto.
	if before == nil {
		candles = s.freshen(symbol, candles, timeframe, bars)
	}
	return candles, nil
}

// freshen pliega el M1 recien llegado (RecentCache) al mismo bucket que el
// timeframe pedido -- mismo plegado que ya usa GetCurrentCandle para la
// vela en formacion, aplicado aca tambien a los buckets ya cerrados. No
// hace falta un cache por timeframe: M1 en RecentCache ya tiene todo lo
// necesario para armar cualquier derivado al vuelo (ver RecentCache.RangeAggregated).
func (s *getCandlesService) freshen(symbol string, base []domain.Candle, timeframe domain.Timeframe, bars int) []domain.Candle {
	bucket, err := bucketDuration(timeframe)
	if err != nil {
		return base // timeframe de calendario (semana/mes/anio): RecentCache jamas alcanza a cubrir tanto
	}
	oldestCached, hasCached := s.recentCache.OldestCovered(symbol)
	if !hasCached {
		return base
	}
	// Un bucket cuyo INICIO cae antes de lo que el cache alcanza a cubrir
	// (H1+ con 20 M1 en cache: nunca cubre un bucket entero) esta armado con
	// M1 incompleto -- le falta la parte de atras, que ya salio de la
	// ventana del cache. Ni se usa la version del cache para ESE bucket, ni
	// se descarta el de la BD: se deja tal cual venia, mismo comportamiento
	// que tenia antes de este cambio. Solo los buckets que el cache cubre
	// DESDE SU PROPIO INICIO son seguros de reconstruir enteros.
	firstFullBucketStart := oldestCached.Truncate(bucket)
	if firstFullBucketStart.Before(oldestCached) {
		firstFullBucketStart = firstFullBucketStart.Add(bucket)
	}
	kept := make([]domain.Candle, 0, len(base))
	for _, c := range base {
		if c.Timestamp.Before(firstFullBucketStart) {
			kept = append(kept, c)
		}
	}
	fresh := s.recentCache.RangeAggregated(symbol, firstFullBucketStart, time.Now().Add(time.Second), bucket, timeframe)
	merged := append(kept, fresh...)
	if len(merged) > bars {
		merged = merged[len(merged)-bars:]
	}
	return merged
}

// bucketDuration: Duration() cubre M1/H1/D1 (base, ya con duracion fija);
// Aggregation() cubre los derivados (M5, M15...). Sin ninguna de las dos
// (semana/mes/anio, calendario) freshen se lo salta -- RecentCache retiene
// unas pocas decenas de barras M1, nunca alcanza a cubrir nada de ese tamano.
func bucketDuration(tf domain.Timeframe) (time.Duration, error) {
	if d, err := tf.Duration(); err == nil {
		return d, nil
	}
	if _, _, approx, ok := tf.Aggregation(); ok && approx > 0 {
		return approx, nil
	}
	return 0, fmt.Errorf("no hay duracion de bucket fija para %s", tf)
}

// candlesBatchFallbackWorkers: concurrencia acotada para el camino
// per-simbolo (timeframes derivados sin continuous aggregate propio, o si
// el batch agregado fallo) -- mismo criterio que liveRolloutWorkers, no
// saturar el pool de conexiones con miles de queries de golpe.
const candlesBatchFallbackWorkers = 4

// GetCandlesBatch resuelve TODO el lote en una sola consulta cuando el
// timeframe es derivado (ver domain.Timeframe.Aggregation) -- agrega el
// timeframe base on-the-fly via out.CandleRepository.GetSeriesAggregatedBatch
// en vez de depender de un continuous aggregate materializado (retirado:
// solo cubria M5/M15, y su politica de refresco no backfillea historia
// vieja -- confirmado en vivo el 2026-08-23 como causa de un hueco real de
// datos). Antes de este cambio, el resto de timeframes derivados (todos
// menos M5/M15) no tenian ningun camino en lote y siempre caian al
// per-simbolo; ahora lo tienen todos por igual.
//
// Consulta primero el cache per-simbolo (mismo candleCache de GetCandles,
// misma key) -- signal-processing repite el batch del universo completo con
// alta superposicion de simbolos entre corridas, asi que solo los simbolos
// faltantes pagan la agregacion. Si el batch agregado da error, el resto
// (los que ya estaban en el cache no necesitan ni eso) cae al per-simbolo
// de siempre.
func (s *getCandlesService) GetCandlesBatch(ctx context.Context, symbols []string, timeframe domain.Timeframe, bars int) map[string][]domain.Candle {
	result := make(map[string][]domain.Candle, len(symbols))
	symbolByKey := make(map[string]string, len(symbols))
	missingKeys := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		key := candleCacheKey(symbol, timeframe, bars, nil)
		if candles, ok := s.cache.get(key); ok {
			// GetCandlesBatch no tiene parametro `before` -- toda llamada es
			// "hasta ahora", asi que el cache de 60s (bulk historico, cacheable
			// sin riesgo) siempre se refresca con RecentCache antes de servirse.
			result[symbol] = s.freshen(symbol, candles, timeframe, bars)
			continue
		}
		symbolByKey[key] = symbol
		missingKeys = append(missingKeys, key)
	}
	if len(missingKeys) == 0 {
		return result
	}

	// claim/join (ver batchCoalescer): si otro GetCandlesBatch concurrente
	// (otro escaner, u otro caller) ya esta pidiendo alguno de estos mismos
	// simbolos ahora mismo, este caller no repite esa consulta -- espera el
	// resultado en vez de duplicarla.
	ownKeys, joinKeys := s.coalescer.claim(missingKeys)
	ownSymbols := make([]string, len(ownKeys))
	for i, key := range ownKeys {
		ownSymbols[i] = symbolByKey[key]
	}

	owned := s.fetchAndCacheMissing(ctx, ownSymbols, timeframe, bars)
	for symbol, candles := range owned {
		result[symbol] = candles
	}
	s.coalescer.resolve(ownKeys, symbolByKey, owned)

	for key, pf := range joinKeys {
		<-pf.done
		if pf.hasData {
			result[symbolByKey[key]] = pf.candles
		}
	}
	return result
}

// fetchAndCacheMissing resuelve `missing` (ya filtrado por cache y por
// coalescer -- solo lo que este caller reclamo de verdad) con el camino de
// siempre: lote agregado/base en una sola consulta, o per-simbolo si ambos
// fallan. Deja todo cacheado antes de devolver.
func (s *getCandlesService) fetchAndCacheMissing(ctx context.Context, missing []string, timeframe domain.Timeframe, bars int) map[string][]domain.Candle {
	result := make(map[string][]domain.Candle, len(missing))
	if len(missing) == 0 {
		return result
	}

	if source, bucket, approxPeriod, ok := timeframe.Aggregation(); ok {
		if batch, err := s.repo.GetSeriesAggregatedBatch(ctx, missing, timeframe, source, bucket, approxPeriod, bars); err == nil {
			return s.absorbBatch(missing, batch, timeframe, bars, result)
		}
	} else if batch, err := s.repo.GetSeries(ctx, missing, timeframe, bars); err == nil {
		// Timeframe base (M1/D1): GetSeries ya resuelve el lote completo en
		// una sola consulta (ver CandleRepository.GetSeries), igual que
		// GetSeriesAggregatedBatch para los derivados -- sin esta rama, todo
		// batch de un timeframe base caia siempre a getCandlesBatchPerSymbol
		// (4 workers, 2 round trips POR SIMBOLO), que con lotes de miles de
		// simbolos (ej. RANGE_EXTREME_PROXIMITY en D1) tardaba mas que el
		// timeout de 90s del cliente. Confirmado en vivo el 2026-09-11.
		return s.absorbBatch(missing, batch, timeframe, bars, result)
	}

	for symbol, candles := range s.getCandlesBatchPerSymbol(ctx, missing, timeframe, bars) {
		result[symbol] = candles
	}
	return result
}

// absorbBatch aplica al resultado de un batch (agregado o base) el mismo
// tratamiento: cachear cada serie (incluida vacia, para no volver a pagar la
// consulta en cada llamada -- ver comentario historico sobre el ~9.7% del
// universo sin dato M1 real) y aplicar freshen antes de devolver.
func (s *getCandlesService) absorbBatch(
	missing []string, batch map[string][]domain.Candle, timeframe domain.Timeframe, bars int, result map[string][]domain.Candle,
) map[string][]domain.Candle {
	for symbol, candles := range batch {
		s.cache.put(candleCacheKey(symbol, timeframe, bars, nil), candles)
		result[symbol] = s.freshen(symbol, candles, timeframe, bars)
	}
	for _, symbol := range missing {
		if _, ok := batch[symbol]; !ok {
			s.cache.put(candleCacheKey(symbol, timeframe, bars, nil), []domain.Candle{})
			if fresh := s.freshen(symbol, nil, timeframe, bars); len(fresh) > 0 {
				result[symbol] = fresh
			}
		}
	}
	return result
}

func (s *getCandlesService) getCandlesBatchPerSymbol(ctx context.Context, symbols []string, timeframe domain.Timeframe, bars int) map[string][]domain.Candle {
	result := make(map[string][]domain.Candle, len(symbols))
	var mu sync.Mutex

	jobs := make(chan string, len(symbols))
	for _, sym := range symbols {
		jobs <- sym
	}
	close(jobs)

	var wg sync.WaitGroup
	for i := 0; i < candlesBatchFallbackWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for symbol := range jobs {
				candles, err := s.GetCandles(ctx, symbol, timeframe, bars, nil)
				if err != nil || len(candles) == 0 {
					continue
				}
				mu.Lock()
				result[symbol] = candles
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return result
}

func candleCacheKey(symbol string, timeframe domain.Timeframe, bars int, before *time.Time) string {
	beforeKey := ""
	if before != nil {
		beforeKey = strconv.FormatInt(before.UnixMilli(), 10)
	}
	return symbol + "|" + string(timeframe) + "|" + strconv.Itoa(bars) + "|" + beforeKey
}
