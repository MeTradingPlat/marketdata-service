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
}

func NewGetCandlesService(repo out.CandleRepository, recentCache *livecandles.RecentCache) in.GetCandlesService {
	return &getCandlesService{repo: repo, cache: candleCache{entries: make(map[string]candleCacheEntry)}, recentCache: recentCache}
}

func (s *getCandlesService) GetCandles(ctx context.Context, symbol string, timeframe domain.Timeframe, bars int, before *time.Time) ([]domain.Candle, error) {
	// "M1 hasta ahora" (before=nil) es exactamente lo que RecentCache cubre
	// en vivo -- saltarse el cache de 60s aca evita devolver el mismo
	// volumen incompleto varias veces seguidas mientras la fila recien
	// cerrada todavia no es visible en Postgres (confirmado en vivo el
	// 2026-08-27 con EMAT). Una fecha puntual (before != nil) es historico
	// ya cerrado, sigue por el camino de siempre.
	if timeframe == domain.M1 && before == nil {
		return s.getFreshM1(ctx, symbol, bars)
	}
	key := candleCacheKey(symbol, timeframe, bars, before)
	if candles, ok := s.cache.get(key); ok {
		return candles, nil
	}
	candles, err := s.repo.GetCandles(ctx, symbol, timeframe, bars, before)
	if err != nil {
		return nil, fmt.Errorf("getting candles for %s %s: %w", symbol, timeframe, err)
	}
	s.cache.put(key, candles)
	return candles, nil
}

// getFreshM1 combina Postgres (todo lo anterior a lo que RecentCache ya
// cubre) con RecentCache (la cola reciente, nunca mas atrasada que el
// ultimo tick procesado) -- sin solape: el limite superior de la consulta a
// Postgres es exclusivo justo donde arranca lo cacheado.
func (s *getCandlesService) getFreshM1(ctx context.Context, symbol string, bars int) ([]domain.Candle, error) {
	oldestCached, hasCached := s.recentCache.OldestCovered(symbol)
	if !hasCached {
		candles, err := s.repo.GetCandles(ctx, symbol, domain.M1, bars, nil)
		if err != nil {
			return nil, fmt.Errorf("getting candles for %s M1: %w", symbol, err)
		}
		return candles, nil
	}
	older, err := s.repo.GetCandles(ctx, symbol, domain.M1, bars, &oldestCached)
	if err != nil {
		return nil, fmt.Errorf("getting candles for %s M1: %w", symbol, err)
	}
	recent := s.recentCache.Range(symbol, oldestCached, time.Now().Add(time.Second))
	merged := append(older, recent...)
	if len(merged) > bars {
		merged = merged[len(merged)-bars:]
	}
	return merged, nil
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
	missing := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		// M1 nunca lee del cache de 60s aca -- ver el comentario de
		// getFreshM1: cae al camino per-simbolo de abajo, que si lo usa.
		if timeframe != domain.M1 {
			if candles, ok := s.cache.get(candleCacheKey(symbol, timeframe, bars, nil)); ok {
				result[symbol] = candles
				continue
			}
		}
		missing = append(missing, symbol)
	}
	if len(missing) == 0 {
		return result
	}

	if source, bucket, approxPeriod, ok := timeframe.Aggregation(); ok {
		if batch, err := s.repo.GetSeriesAggregatedBatch(ctx, missing, timeframe, source, bucket, approxPeriod, bars); err == nil {
			for symbol, candles := range batch {
				s.cache.put(candleCacheKey(symbol, timeframe, bars, nil), candles)
				result[symbol] = candles
			}
			// Un simbolo sin filas en el batch (sin dato M1 real, ~9.7% del
			// universo NYSE+NASDAQ+AMEX confirmado en vivo el 2026-08-23) no
			// aparece en batch y sin este cacheo quedaba "missing" para
			// siempre -- pagando el JOIN LATERAL de nuevo en CADA llamada,
			// sin ningun beneficio del cache. Mismo comportamiento que ya
			// tiene GetCandles (put incondicional aunque candles sea vacio).
			for _, symbol := range missing {
				if _, ok := batch[symbol]; !ok {
					s.cache.put(candleCacheKey(symbol, timeframe, bars, nil), []domain.Candle{})
				}
			}
			return result
		}
	}

	for symbol, candles := range s.getCandlesBatchPerSymbol(ctx, missing, timeframe, bars) {
		result[symbol] = candles
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
