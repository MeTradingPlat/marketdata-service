package ingestion

import (
	"context"
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
	"golang.org/x/sync/singleflight"
)

type getCandlesService struct {
	repo        out.CandleRepository
	recentCache *livecandles.RecentCache
	// fetchGroup colapsa pedidos concurrentes de la MISMA clave (simbolo +
	// timeframe + bars + before) en una sola consulta real -- sin esto, dos
	// llamadas que llegan al mismo tiempo (ej. dos pestañas del frontend
	// mirando el mismo simbolo, o un escaner y un pivots pidiendo D1 de AAPL
	// a la vez) pagan la misma consulta a Postgres por duplicado -- mismo patron ya usado en oauth.go para el
	// mismo tipo de problema (confirmado en vivo alla: llamadas concurrentes
	// duplicando trabajo real).
	fetchGroup singleflight.Group
	// coalescer: mismo motivo que fetchGroup, para GetCandlesBatch -- ver
	// batchCoalescer.
	coalescer *batchCoalescer
}

func NewGetCandlesService(repo out.CandleRepository, recentCache *livecandles.RecentCache) in.GetCandlesService {
	return &getCandlesService{
		repo: repo, recentCache: recentCache, coalescer: newBatchCoalescer(),
	}
}

func (s *getCandlesService) GetCandles(ctx context.Context, symbol string, timeframe domain.Timeframe, bars int, before *time.Time) ([]domain.Candle, error) {
	key := candleCacheKey(symbol, timeframe, bars, before)
	fetched, err, _ := s.fetchGroup.Do(key, func() (interface{}, error) {
		return s.repo.GetCandles(ctx, symbol, timeframe, bars, before)
	})
	if err != nil {
		return nil, fmt.Errorf("getting candles for %s %s: %w", symbol, timeframe, err)
	}
	candles := fetched.([]domain.Candle)
	// "hasta ahora" (before=nil): la cola de lo que RecentCache ya cubre se
	// reemplaza por su version agregada en vivo -- Postgres no garantiza que
	// la fila mas reciente ya sea visible/completa (confirmado
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
