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

const candleCacheMaxEntries = 20000

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
		for k := range c.entries {
			delete(c.entries, k)
			break
		}
	}
	c.entries[key] = candleCacheEntry{expires: time.Now().Add(candleCacheTTL), candles: candles}
}

type getCandlesService struct {
	repo  out.CandleRepository
	cache candleCache
}

func NewGetCandlesService(repo out.CandleRepository) in.GetCandlesService {
	return &getCandlesService{repo: repo, cache: candleCache{entries: make(map[string]candleCacheEntry)}}
}

func (s *getCandlesService) GetCandles(ctx context.Context, symbol string, timeframe domain.Timeframe, bars int, before *time.Time) ([]domain.Candle, error) {
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

// GetCandlesFresh: ver el puerto (ports/in/services.go) -- mismo resultado
// sin pasar por el cache.
func (s *getCandlesService) GetCandlesFresh(ctx context.Context, symbol string, timeframe domain.Timeframe, bars int, before *time.Time) ([]domain.Candle, error) {
	candles, err := s.repo.GetCandles(ctx, symbol, timeframe, bars, before)
	if err != nil {
		return nil, fmt.Errorf("getting candles for %s %s: %w", symbol, timeframe, err)
	}
	return candles, nil
}

func candleCacheKey(symbol string, timeframe domain.Timeframe, bars int, before *time.Time) string {
	beforeKey := ""
	if before != nil {
		beforeKey = strconv.FormatInt(before.UnixMilli(), 10)
	}
	return symbol + "|" + string(timeframe) + "|" + strconv.Itoa(bars) + "|" + beforeKey
}
