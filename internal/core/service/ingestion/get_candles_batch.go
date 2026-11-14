package ingestion

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

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
// Los pedidos concurrentes de los mismos simbolos se juntan en
// batchCoalescer; ninguna serie se guarda despues de responder. Si el batch
// agregado da error, cae al per-simbolo de siempre.
func (s *getCandlesService) GetCandlesBatch(ctx context.Context, symbols []string, timeframe domain.Timeframe, bars int) map[string][]domain.Candle {
	result := make(map[string][]domain.Candle, len(symbols))
	symbolByKey := make(map[string]string, len(symbols))
	missingKeys := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		key := candleCacheKey(symbol, timeframe, bars, nil)
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
		result[symbol] = s.freshen(symbol, candles, timeframe, bars)
	}
	for _, symbol := range missing {
		if _, ok := batch[symbol]; !ok {
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
