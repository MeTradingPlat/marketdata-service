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

// StartUniverseCycle corre el ciclo completo del universo -- D1 fase 1, H1
// fase 2, M1 fase 3 -- una vez al arrancar (nada esta en vivo todavia, no
// hace falta desconectar nada) y despues en cada ventana de mantenimiento
// (mercado cerrado): ahi si cierra todas las conexiones DxLink primero
// (ver CandlePool.CloseAllConnections, tambien usado entre D1->H1 y H1->M1
// dentro de RunSweep) para que cada fase arranque con cero sesiones
// abiertas ante TastyTrade, y vuelve a suscribir M1 al terminar -- cada
// simbolo retoma desde su propio watermark, sin hueco real porque el
// mercado estuvo cerrado.
//
// catchup.RefreshFundamentals (REST a /market-data/by-type y
// /market-metrics) corre acotado a un piloto de 10 simbolos por mercado
// (ver topSymbolsPerMarket) despues del rollout M1 -- REST puro, no compite
// por conexiones DxLink con las fases de velas.
func StartUniverseCycle(ctx context.Context, cfg *configs.Config, gateway out.MarketDataGateway, symbols out.SymbolRepository, candles out.CandleRepository, fundamentals out.FundamentalsRepository, ingest in.IngestCandlesService) {
	go func() {
		runUniverseCycle(ctx, cfg, gateway, symbols, candles, fundamentals, ingest, true)
		for {
			wait := time.Until(catchup.NextMaintenanceWindowAt(time.Now()))
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
				runUniverseCycle(ctx, cfg, gateway, symbols, candles, fundamentals, ingest, false)
			}
		}
	}()
}

func runUniverseCycle(ctx context.Context, cfg *configs.Config, gateway out.MarketDataGateway, symbols out.SymbolRepository, candles out.CandleRepository, fundamentals out.FundamentalsRepository, ingest in.IngestCandlesService, firstRun bool) {
	if !firstRun {
		gateway.ResetLiveConnections()
	}

	tracked := catchup.RunSweep(ctx, gateway, symbols, candles, ingest, cfg.SweepWorkers)
	if len(tracked) == 0 {
		log.Error().Msg("universe sweep returned no symbols, skipping live M1 rollout")
		return
	}

	startLiveUniverse(ctx, ingest, tracked)
	catchup.RefreshFundamentals(ctx, gateway, symbols, fundamentals)
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
