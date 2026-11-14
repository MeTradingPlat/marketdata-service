package main

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/intraday"
	"github.com/rs/zerolog/log"
)

const liveRolloutWorkers = 20

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
				waitForSaveBacklog(ctx, ingest)
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

// seedSnapshotTracker carga la sesion de hoy para todo el universo en UNA
// sola consulta de lote (el mismo costo que antes pagaba CADA request de
// fundamentals/realtime, ver el comentario de GetSnapshotsBatch) -- corre
// una vez por ventana de mantenimiento, no en el camino caliente. Un error
// aca no frena el arranque: GetSnapshotsBatch sigue cubriendo lo que falte
// via su propio fallback a BD por-simbolo (pero solo para ESA respuesta, no
// persiste en el tracker -- de ahi el reintento aca).
func seedSnapshotTracker(ctx context.Context, candles out.CandleRepository, tracker *intraday.SnapshotTracker, tracked []domain.Symbol) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		log.Error().Err(err).Msg("seeding snapshot tracker: loading America/New_York failed")
		return
	}
	nowET := time.Now().In(loc)
	day := time.Date(nowET.Year(), nowET.Month(), nowET.Day(), 0, 0, 0, 0, loc)

	symbols := make([]string, len(tracked))
	for i, s := range tracked {
		symbols[i] = s.Symbol
	}

	start := time.Now()
	snapshots, err := candles.GetIntradaySessionsBatch(ctx, symbols)
	if err != nil {
		log.Error().Err(err).Msg("seeding snapshot tracker failed, falling back to per-request DB reads")
		return
	}

	tracker.Seed(day, symbols, snapshots)
	log.Info().Int("symbols", len(snapshots)).Dur("elapsed", time.Since(start)).Msg("snapshot tracker seeded")

	// SeedLastClose por separado: sin esto, el fallback de precio actual
	// (GetSnapshotsBatch, ver LastClose) no tendria nada que devolver para
	// ningun simbolo hasta su primer tick en vivo, y caeria de nuevo en la
	// consulta lenta para el universo ENTERO justo tras este mismo
	// despliegue.
	lastStart := time.Now()
	lastCandles, err := candles.GetSeries(ctx, symbols, domain.M1, 1)
	if err != nil {
		log.Error().Err(err).Msg("seeding last-close tracker failed, falling back to per-request DB reads")
		return
	}
	lastClose := make(map[string]domain.Candle, len(lastCandles))
	for symbol, bars := range lastCandles {
		if len(bars) > 0 {
			lastClose[symbol] = bars[len(bars)-1]
		}
	}
	tracker.SeedLastClose(symbols, lastClose)
	log.Info().Int("symbols", len(lastClose)).Dur("elapsed", time.Since(lastStart)).Msg("last-close tracker seeded")
}
