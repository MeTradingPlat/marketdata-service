package catchup

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	fundamentalscache "github.com/MeTradingPlat/marketdata-service/internal/core/service/fundamentals"
	"github.com/rs/zerolog/log"
)

// openInterestWorkers: el endpoint de open interest de TastyTrade consulta
// la cadena /nested y luego lotes de 100 contratos en /market-data/by-type
// por simbolo, asi que un pool moderado evita saturar el rate limit de la
// API externa.
const openInterestWorkers = 4

// RefreshOpenInterest consulta TastyTrade para los simbolos cuyo open
// interest no se calculo en la ventana de mantenimiento actual, guarda el
// total en Postgres (columna open_interest de dividends) y actualiza
// FundamentalsCache en memoria para que GetSymbolDetails responda en 0ms.
func RefreshOpenInterest(ctx context.Context, gateway out.OpenInterestGateway, fundamentalsRepo out.FundamentalsRepository, fundamentalsCache *fundamentalscache.FundamentalsCache, windowStart time.Time) error {
	stale, err := fundamentalsRepo.GetSymbolsWithStaleOpenInterest(ctx, windowStart)
	if err != nil {
		return fmt.Errorf("selecting symbols with stale open interest: %w", err)
	}
	if len(stale) == 0 {
		return nil
	}

	start := time.Now()
	jobs := make(chan string, len(stale))
	for _, s := range stale {
		jobs <- s
	}
	close(jobs)

	var (
		mu            sync.Mutex
		found         = make(map[string]float64)
		attemptedOnly []string
		errCount      int
		lastErr       error
		wg            sync.WaitGroup
	)

	for i := 0; i < openInterestWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for symbol := range jobs {
				if ctx.Err() != nil {
					return
				}
				val, ok, err := gateway.OpenInterest(ctx, symbol)
				mu.Lock()
				if err != nil {
					errCount++
					lastErr = err
				} else if ok && val > 0 {
					found[symbol] = val
				} else {
					attemptedOnly = append(attemptedOnly, symbol)
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if err := fundamentalsRepo.UpsertOpenInterestBatch(ctx, found, attemptedOnly); err != nil {
		return fmt.Errorf("upserting open interest batch: %w", err)
	}
	fundamentalsCache.MergeOpenInterest(found, attemptedOnly)

	if errCount > 0 {
		log.Warn().
			Err(lastErr).
			Int("checked", len(stale)).
			Int("found", len(found)).
			Int("errors", errCount).
			Dur("elapsed", time.Since(start)).
			Msg("open interest refresh finished with errors")
	} else {
		log.Info().
			Int("checked", len(stale)).
			Int("found", len(found)).
			Dur("elapsed", time.Since(start)).
			Msg("open interest refresh finished")
	}
	return nil
}
