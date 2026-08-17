package main

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/catchup"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs"
	"github.com/rs/zerolog/log"
)

const liveRolloutWorkers = 20

const (
	refreshMaxAttempts = 3
	refreshRetryDelay  = 3 * time.Minute
)

// refreshWithRetry reintenta un refresh idempotente del barrido nocturno --
// confirmado en vivo: postgres cayo en recovery mode justo durante el upsert
// de beta (SQLSTATE 57P03) y con un solo intento se perdieron beta, earnings
// y el refresh externo de SEC/FINRA hasta la ventana del dia siguiente. Un
// fallo transitorio de BD no deberia costar una noche entera de datos.
func refreshWithRetry(name string, fn func() error) {
	for attempt := 1; attempt <= refreshMaxAttempts; attempt++ {
		if err := fn(); err != nil {
			log.Error().Err(err).Int("attempt", attempt).Str("refresh", name).Msg("nightly refresh failed")
			if attempt < refreshMaxAttempts {
				time.Sleep(refreshRetryDelay)
			}
			continue
		}
		return
	}
}

// StartUniverseCycle corre el ciclo completo del universo una vez al
// arrancar y despues en cada ventana de mantenimiento (mercado cerrado).
//
// Arranque en frio: M1 en vivo PRIMERO (retoma desde el watermark de cada
// simbolo con replay de lo perdido -- velas al dia en ~1 min sin importar
// cuanto estuvo caido el servicio), despues el barrido D1/H1 en lotes
// compartiendo el pool, y el resto de refrescos. Ventana de mantenimiento:
// cierra todas las conexiones DxLink primero (ver CandlePool.
// CloseAllConnections, tambien usado entre D1->H1 y H1->M1 dentro de
// RunSweep) para que cada fase arranque con cero sesiones abiertas ante
// TastyTrade, y vuelve a suscribir M1 al terminar -- cada simbolo retoma
// desde su propio watermark, sin hueco real porque el mercado estuvo
// cerrado.
//
// catchup.RefreshFundamentals (REST a /market-data/by-type y
// /market-metrics) corre acotado a un piloto de 10 simbolos por mercado
// (ver topSymbolsPerMarket) despues del rollout M1 -- REST puro, no compite
// por conexiones DxLink con las fases de velas.
func StartUniverseCycle(ctx context.Context, cfg *configs.Config, gateway out.MarketDataGateway, symbols out.SymbolRepository, candles out.CandleRepository, fundamentals out.FundamentalsRepository, ingest in.IngestCandlesService, edgar out.SharesOutstandingGateway, insiders out.InsiderOwnershipGateway, finra out.ShortInterestGateway, profile out.ProfileSharesGateway) {
	go func() {
		runUniverseCycle(ctx, cfg, gateway, symbols, candles, fundamentals, ingest, edgar, insiders, finra, profile, true)
		for {
			wait := time.Until(catchup.NextMaintenanceWindowAt(time.Now()))
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
				runUniverseCycle(ctx, cfg, gateway, symbols, candles, fundamentals, ingest, edgar, insiders, finra, profile, false)
			}
		}
	}()
}

func runUniverseCycle(ctx context.Context, cfg *configs.Config, gateway out.MarketDataGateway, symbols out.SymbolRepository, candles out.CandleRepository, fundamentals out.FundamentalsRepository, ingest in.IngestCandlesService, edgar out.SharesOutstandingGateway, insiders out.InsiderOwnershipGateway, finra out.ShortInterestGateway, profile out.ProfileSharesGateway, firstRun bool) {
	tracked := catchup.ReconcileAndTracked(ctx, gateway, symbols)
	if len(tracked) == 0 {
		log.Error().Msg("universe sweep returned no symbols, skipping live M1 rollout")
		return
	}

	// Arranque en frio: M1 en vivo PRIMERO -- cada simbolo retoma desde su
	// watermark y dxLink hace replay de lo perdido, asi que las velas al dia
	// vuelven en ~1 min sin importar cuanto tiempo estuvo caido el servicio
	// (a pedido explicito del usuario: un deploy/restart no puede dejar M1
	// abajo mientras corre el barrido pesado). El barrido D1/H1 corre
	// DESPUES, en lotes, compartiendo el pool con el streaming sin cerrar
	// conexiones (resetBetweenPhases=false).
	if firstRun {
		startLiveUniverse(ctx, ingest, tracked)
	} else {
		gateway.ResetLiveConnections()
	}

	catchup.RunSweep(ctx, gateway, candles, ingest, tracked, cfg.SweepWorkers, !firstRun)

	if !firstRun {
		startLiveUniverse(ctx, ingest, tracked)
	}
	catchup.RefreshTradingStatus(ctx, gateway, symbols, fundamentals)
	catchup.RefreshMarketMetrics(ctx, gateway, symbols, fundamentals)
	// RefreshBeta va despues de RefreshMarketMetrics: este acaba de escribir
	// los betas de TastyTrade, y el nuestro pasa por encima solo donde se
	// pudo calcular (5Y monthly desde velas propias, ver domain.MonthlyBeta).
	refreshWithRetry("beta", func() error {
		return catchup.RefreshBeta(ctx, gateway, candles, symbols, fundamentals)
	})
	// RefreshEarningsHistory va DESPUES de RefreshMarketMetrics: este es el
	// que pisa next_earnings_date con el dato vigente de TastyTrade, asi que
	// el lote de "vencidos o nunca buscados" que queda despues es chico (solo
	// emisores cuyo earnings ya paso o que TastyTrade no cubre) -- el
	// COALESCE del upsert nunca pisa una fecha vigente con una prediccion.
	refreshWithRetry("earnings history", func() error {
		return catchup.RefreshEarningsHistory(ctx, gateway, fundamentals)
	})

	// En background: descarga+parseo del companyfacts.zip de SEC EDGAR
	// (~1.5GB, hasta 20 min la primera vez del dia) y de los ZIPs
	// trimestrales de insiders no deben demorar el arranque de la ventana
	// de mantenimiento ni bloquear la siguiente vuelta del ciclo -- mismo
	// patron que CompletableFuture.runAsync en la version Java.
	go refreshWithRetry("external fundamentals", func() error {
		return catchup.RefreshExternalFundamentals(ctx, edgar, insiders, finra, profile, symbols, fundamentals)
	})
}

// startLiveUniverse suscribe M1 en vivo para todo el universo con un pool
// acotado de workers -- 13k intentos de golpe abrirían un aluvión de
// handshakes DxLink simultaneos nunca probado a este tamaño; con workers
// limitados se reparte en el tiempo, y cada fallo puntual ya tiene su
// propio reintento (startLiveWithRetry).
func startLiveUniverse(ctx context.Context, ingest in.IngestCandlesService, tracked []domain.Symbol) {
	jobs := make(chan string, len(tracked))
	for _, s := range tracked {
		jobs <- s.Symbol
	}
	close(jobs)

	start := time.Now()
	done := make(chan struct{})
	for i := 0; i < liveRolloutWorkers; i++ {
		go func() {
			for symbol := range jobs {
				startLiveWithRetry(ctx, ingest, symbol)
			}
			done <- struct{}{}
		}()
	}
	for i := 0; i < liveRolloutWorkers; i++ {
		<-done
	}

	log.Info().Int("symbols", len(tracked)).Dur("elapsed", time.Since(start)).Msg("live M1 rollout finished")
}
