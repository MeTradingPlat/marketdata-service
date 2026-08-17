package catchup

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

// betaWorkers: el calculo es CPU+DB por simbolo (una query D1 de ~1300
// barras cada uno), un pool chico alcanza sin ahogar al host compartido --
// bajado de 20 a 8 tras confirmar en vivo que 20 workers concurrentes
// sobre la hypertable mas el resto del barrido empujaban a postgres al
// tope de memoria (4GiB) hasta reiniciarse en recovery mode a mitad del
// upsert, matando tambien earnings y el refresh externo.
const betaWorkers = 8

const providerBetaChunk = 1000

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
// historia conservan el beta del proveedor, salvo que este no traiga beta
// (0): ahi entra el fallback de 12 meses (beta 1Y). Corre en el barrido
// nocturno despues de RefreshMarketMetrics (que acaba de escribir los betas
// de TastyTrade): este pasa por encima solo con el valor mejor.
func RefreshBeta(ctx context.Context, candles out.CandleRepository, symbols out.SymbolRepository, fundamentalsRepo out.FundamentalsRepository) error {
	tracked, err := symbols.Tracked(ctx)
	if err != nil {
		return fmt.Errorf("listing tracked symbols: %w", err)
	}

	marketCandles, err := candles.GetCandles(ctx, betaMarketProxy, domain.D1, betaHistoryBars, nil)
	if err != nil || len(marketCandles) < betaHistoryBars/2 {
		return fmt.Errorf("loading market proxy history for %s: %w", betaMarketProxy, err)
	}

	start := time.Now()
	jobSymbols := make([]string, len(tracked))
	for i, s := range tracked {
		jobSymbols[i] = s.Symbol
	}

	// El beta del proveedor se lee una sola vez para el universo entero,
	// en lotes de a mil -- una sola consulta con los 13k simbolos en el IN
	// aportaba pico de memoria a postgres justo cuando los workers arrancan
	// (confirmado en vivo: recovery mode a mitad del barrido).
	providerBeta := map[string]float64{}
	for i := 0; i < len(jobSymbols); i += providerBetaChunk {
		end := min(i+providerBetaChunk, len(jobSymbols))
		if fundBySymbol, ferr := fundamentalsRepo.GetBatch(ctx, jobSymbols[i:end]); ferr == nil {
			for symbol, f := range fundBySymbol {
				providerBeta[symbol] = f.Beta
			}
		}
	}

	jobs := make(chan string, len(jobSymbols))
	for _, symbol := range jobSymbols {
		jobs <- symbol
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
				if beta == nil && providerBeta[symbol] == 0 {
					// Fallback beta 1Y con la mejor serie disponible -- un beta
					// real de 12 meses es mejor que N/A para los simbolos
					// cortos SIN beta del proveedor. Con beta real del
					// proveedor se conserva el suyo.
					beta = domain.MonthlyBetaMin(symbolCandles, marketCandles, domain.BetaFallbackMonths)
				}
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
		return fmt.Errorf("upserting beta batch: %w", err)
	}
	log.Info().Int("symbols", len(updates)).Dur("elapsed", time.Since(start)).Msg("beta refresh finished")
	return nil
}
