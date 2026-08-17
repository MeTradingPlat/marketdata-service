package catchup

import (
	"context"
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

// betaWorkers: el calculo es CPU+DB por simbolo (una query D1 de ~1300
// barras cada uno), un pool chico alcanza sin ahogar al host compartido.
const betaWorkers = 20

// betaHistoryBars: 5 anios de velas D1 (~252 por anio) es la convencion
// estandar para beta mensual (ver stockanalysis/Yahoo), y deja meses de
// sobra para los 24 minimos que exige MonthlyBeta.
const betaHistoryBars = 1300

// betaMarketProxy: el proxy de mercado para beta en la practica de la
// industria (SPY replica al S&P 500) -- ya lo tenemos rastreado e
// ingiriendo velas D1, no hace falta otra fuente.
const betaMarketProxy = "SPY"

// RefreshBeta recalcula beta para todo el universo desde las velas D1
// propias (5Y monthly contra SPY, ver domain.MonthlyBeta) y pisa el de
// TastyTrade SOLO donde se pudo calcular -- los simbolos sin 2 anios de
// historia conservan el beta del proveedor. Corre en el barrido nocturno
// despues de RefreshMarketMetrics (que acaba de escribir los betas de
// TastyTrade): este pasa por encima solo con el valor mejor.
func RefreshBeta(ctx context.Context, candles out.CandleRepository, symbols out.SymbolRepository, fundamentalsRepo out.FundamentalsRepository) {
	tracked, err := symbols.Tracked(ctx)
	if err != nil {
		log.Error().Err(err).Msg("beta refresh: failed to list tracked symbols")
		return
	}

	marketCandles, err := candles.GetCandles(ctx, betaMarketProxy, domain.D1, betaHistoryBars, nil)
	if err != nil || len(marketCandles) < betaHistoryBars/2 {
		log.Error().Err(err).Str("symbol", betaMarketProxy).Msg("beta refresh: failed to load market proxy history")
		return
	}

	start := time.Now()
	jobs := make(chan string, len(tracked))
	for _, s := range tracked {
		jobs <- s.Symbol
	}
	close(jobs)

	var mu sync.Mutex
	var updates []domain.Fundamentals
	var wg sync.WaitGroup
	for i := 0; i < betaWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for symbol := range jobs {
				symbolCandles, err := candles.GetCandles(ctx, symbol, domain.D1, betaHistoryBars, nil)
				if err != nil || len(symbolCandles) == 0 {
					continue
				}
				beta := domain.MonthlyBeta(symbolCandles, marketCandles)
				if beta == nil {
					continue
				}
				mu.Lock()
				updates = append(updates, domain.Fundamentals{Symbol: symbol, Beta: *beta})
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if err := fundamentalsRepo.UpsertBeta(ctx, updates); err != nil {
		log.Error().Err(err).Msg("upserting beta refresh failed")
		return
	}
	log.Info().Int("symbols", len(updates)).Dur("elapsed", time.Since(start)).Msg("beta refresh finished")
}
