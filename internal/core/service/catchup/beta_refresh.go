package catchup

import (
	"context"
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

// providerBetaChunk: lotes de a mil simbolos para la lectura del beta de
// TastyTrade -- una sola consulta con los 13k en el IN aportaba pico de
// memoria a postgres (confirmado en vivo: recovery mode a mitad del barrido).
const providerBetaChunk = 1000

// betaSymbolChunk: las series D1 se leen en lotes de a mil simbolos con
// GetSeries (una query por lote) -- el refresh anterior corria una query de
// ~1300 barras POR SIMBOLO (13k queries): saturaba la CPU de postgres ~40
// min y, con el mercado abierto, las escrituras M1 en vivo hacian fila
// detras de la saturacion (las velas del grafico llegaban con minutos de
// lag). Pico de memoria por lote: ~1000 x 1300 barras, holgado en el
// contenedor.
const betaSymbolChunk = 1000

// betaHistoryBars: 5 anios de velas D1 (~252 por anio) es la convencion
// estandar para beta mensual (ver stockanalysis/Yahoo), y deja meses de
// sobra para los 24 minimos que exige MonthlyBeta.
const betaHistoryBars = 1300

// betaMarketProxy: el proxy de mercado para beta en la practica de la
// industria (SPY replica al S&P 500) -- ya lo tenemos rastreado e
// ingiriendo velas D1, no hace falta otra fuente.
const betaMarketProxy = "SPY"

// RefreshBeta recalcula beta desde las velas D1 propias (5Y monthly contra
// SPY, ver domain.MonthlyBeta) SOLO para los simbolos cuyo beta_updated_at
// quedo fuera de la ventana de mantenimiento actual (guard por-simbolo con
// fecha, ver GetSymbolsWithStaleBeta) -- si todos estan calculados no
// calcula nada. Pisa el beta de TastyTrade solo donde se pudo calcular; los
// simbolos sin 2 anios de historia conservan el del proveedor, salvo que
// este no traiga beta (0): ahi entra el fallback de 12 meses (beta 1Y).
// Corre en el backfill despues del barrido D1 y antes de H1 (ver
// universe_cycle.go), que es cuando el D1 propio esta fresco.
func RefreshBeta(ctx context.Context, candles out.CandleRepository, fundamentalsRepo out.FundamentalsRepository, windowStart time.Time) error {
	stale, err := fundamentalsRepo.GetSymbolsWithStaleBeta(ctx, windowStart)
	if err != nil {
		return fmt.Errorf("listing symbols with stale beta: %w", err)
	}
	if len(stale) == 0 {
		log.Info().Msg("beta refresh: all symbols already done for this maintenance window, skipping")
		return nil
	}

	marketCandles, err := candles.GetCandles(ctx, betaMarketProxy, domain.D1, betaHistoryBars, nil)
	if err != nil || len(marketCandles) < betaHistoryBars/2 {
		return fmt.Errorf("loading market proxy history for %s: %w", betaMarketProxy, err)
	}

	start := time.Now()
	jobSymbols := stale

	// El beta del proveedor se lee una sola vez para el universo entero,
	// en lotes de a mil -- una sola consulta con los 13k simbolos en el IN
	// aportaba pico de memoria a postgres (confirmado en vivo: recovery
	// mode a mitad del barrido).
	providerBeta := map[string]float64{}
	for i := 0; i < len(jobSymbols); i += providerBetaChunk {
		end := min(i+providerBetaChunk, len(jobSymbols))
		if fundBySymbol, ferr := fundamentalsRepo.GetBatch(ctx, jobSymbols[i:end]); ferr == nil {
			for symbol, f := range fundBySymbol {
				providerBeta[symbol] = f.Beta
			}
		}
	}

	// Las series D1 del universo entero se leen en lotes de a mil (ver
	// betaSymbolChunk) y el calculo es CPU puro en memoria: el refresh
	// completo baja de ~40 min a pocos minutos, sin workers y sin
	// saturar postgres.
	var updates []domain.Fundamentals
	for i := 0; i < len(jobSymbols); i += betaSymbolChunk {
		end := min(i+betaSymbolChunk, len(jobSymbols))
		series, err := candles.GetSeries(ctx, jobSymbols[i:end], domain.D1, betaHistoryBars)
		if err != nil {
			return fmt.Errorf("loading D1 series batch: %w", err)
		}
		for symbol, symbolCandles := range series {
			if len(symbolCandles) == 0 {
				continue
			}
			beta := domain.MonthlyBeta(symbolCandles, marketCandles)
			if beta == nil && providerBeta[symbol] == 0 {
				// Escalera de fallbacks para historia corta SIN beta del
				// proveedor (con beta real del proveedor se conserva el
				// suyo): 1Y semanal (52 obs, Bloomberg-style), luego 1Y
				// mensual (12 obs), y por ultimo 6M semanal (26 obs) para
				// los simbolos con apenas 6-11 meses de D1 -- cada escalon
				// es un beta real mejor que N/A (confirmado en vivo con
				// MDXH, ADR con ~19 meses que quedaba sin beta del todo).
				beta = domain.WeeklyBetaMin(symbolCandles, marketCandles, domain.BetaFallbackWeeks)
			}
			if beta == nil && providerBeta[symbol] == 0 {
				beta = domain.MonthlyBetaMin(symbolCandles, marketCandles, domain.BetaFallbackMonths)
			}
			if beta == nil && providerBeta[symbol] == 0 {
				beta = domain.WeeklyBetaMin(symbolCandles, marketCandles, domain.BetaMinWeeks)
			}
			if beta == nil {
				continue
			}
			updates = append(updates, domain.Fundamentals{Symbol: symbol, Beta: *beta})
		}
	}

	if err := fundamentalsRepo.UpsertBeta(ctx, updates); err != nil {
		return fmt.Errorf("upserting beta batch: %w", err)
	}
	log.Info().Int("symbols", len(updates)).Dur("elapsed", time.Since(start)).Msg("beta refresh finished")
	return nil
}
