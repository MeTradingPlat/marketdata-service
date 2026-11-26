package catchup

import (
	"context"
	"fmt"
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
	res := fetchOpenInterestBatch(ctx, gateway, stale)

	if err := fundamentalsRepo.UpsertOpenInterestBatch(ctx, res.found, res.attemptedOnly); err != nil {
		return fmt.Errorf("upserting open interest batch: %w", err)
	}
	fundamentalsCache.MergeOpenInterest(res.found, res.attemptedOnly)
	logOpenInterestSummary(start, len(stale), res)
	return nil
}

func logOpenInterestSummary(start time.Time, checked int, res openInterestBatchResult) {
	if res.errCount > 0 {
		log.Warn().Err(res.lastErr).Int("checked", checked).Int("found", len(res.found)).Int("errors", res.errCount).Dur("elapsed", time.Since(start)).Msg("open interest refresh finished with errors")
		return
	}
	log.Info().Int("checked", checked).Int("found", len(res.found)).Dur("elapsed", time.Since(start)).Msg("open interest refresh finished")
}
