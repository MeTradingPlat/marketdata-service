package main

import (
	"context"
	"sync/atomic"
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
// Orden del barrido (diseno original del usuario, pensado para MINIMIZAR
// carga en el servidor): D1 primero con lotes de 100 simbolos por
// suscripcion; al terminar se desuscribe y se CIERRAN las conexiones para
// asegurarse de que la fase termino; luego H1 con el mismo patron; y por
// ultimo M1, que se queda suscrito para siempre. Cada fase arranca con
// cero sesiones abiertas ante TastyTrade -- confirmado en vivo que
// arrastrar conexiones de una fase a la siguiente puede superar el limite
// de sesiones concurrentes. Cada simbolo retoma desde su propio watermark
// (con replay de lo perdido en M1), sin hueco real de datos.
//
// catchup.RefreshFundamentals (REST a /market-data/by-type y
// /market-metrics) corre acotado a un piloto de 10 simbolos por mercado
// (ver topSymbolsPerMarket) despues del rollout M1 -- REST puro, no compite
// por conexiones DxLink con las fases de velas.
func StartUniverseCycle(ctx context.Context, cfg *configs.Config, gateway out.MarketDataGateway, symbols out.SymbolRepository, candles out.CandleRepository, fundamentals out.FundamentalsRepository, ingest in.IngestCandlesService, edgar out.SharesOutstandingGateway, insiders out.InsiderOwnershipGateway, finra out.ShortInterestGateway, profile out.ProfileSharesGateway, backfilling *atomic.Bool) {
	go func() {
		runUniverseCycle(ctx, cfg, gateway, symbols, candles, fundamentals, ingest, edgar, insiders, finra, profile, backfilling, true)
		for {
			wait := time.Until(catchup.NextMaintenanceWindowAt(time.Now()))
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
				runUniverseCycle(ctx, cfg, gateway, symbols, candles, fundamentals, ingest, edgar, insiders, finra, profile, backfilling, false)
			}
		}
	}()
}

func runUniverseCycle(ctx context.Context, cfg *configs.Config, gateway out.MarketDataGateway, symbols out.SymbolRepository, candles out.CandleRepository, fundamentals out.FundamentalsRepository, ingest in.IngestCandlesService, edgar out.SharesOutstandingGateway, insiders out.InsiderOwnershipGateway, finra out.ShortInterestGateway, profile out.ProfileSharesGateway, backfilling *atomic.Bool, firstRun bool) {
	// Pipeline del backfill (diseno del usuario): D1 primero, se cierran
	// las conexiones, se calcula TODO lo que se calcula con D1 (beta y
	// prevClose, por-simbolo con fecha), luego H1 (se cierra, se calcula lo
	// suyo), y por ultimo M1 que se queda suscrito; recien ahi el sistema
	// abre a peticiones. Durante TODO el backfill el servicio responde 503
	// a peticiones externas (BackfillGate) -- las estrategias nunca leen
	// velas a medio rellenar.
	tracked := catchup.ReconcileAndTracked(ctx, gateway, symbols)
	if len(tracked) == 0 {
		log.Error().Msg("universe sweep returned no symbols, skipping live M1 rollout")
		return
	}

	backfilling.Store(true)
	defer backfilling.Store(false)

	if !firstRun {
		gateway.ResetLiveConnections()
	}

	// FASE 1: D1 + beta (guard por-simbolo, se calcula con D1 propio).
	catchup.RunSweepPhase(ctx, gateway, candles, ingest, tracked, domain.D1, cfg.SweepWorkers)
	windowStart := catchup.LastMaintenanceWindowStart(time.Now())
	// RefreshBeta usa el guard por-simbolo beta_updated_at: solo calcula
	// los simbolos cuyo beta no se calculo en esta ventana de
	// mantenimiento (ver beta_refresh.go).
	refreshWithRetry("beta", func() error {
		return catchup.RefreshBeta(ctx, candles, fundamentals, windowStart)
	})

	// FASE 2: H1, desde cero sesiones (RunSweepPhase cierra al terminar).
	catchup.RunSweepPhase(ctx, gateway, candles, ingest, tracked, domain.H1, cfg.SweepWorkers)

	// FASE 3: M1 en vivo (se queda suscrito) + prevClose (se calcula desde
	// las velas M1 de la sesion anterior, asi que va DESPUES del rollout
	// M1 -- corria antes con la tabla M1 vacia en un refill en frio y
	// calculaba 0) + verificacion de huecos de los ultimos dias (backstop
	// del replay).
	startLiveUniverse(ctx, ingest, tracked)
	refreshWithRetry("prev close", func() error {
		return catchup.RefreshPrevClose(ctx, candles, fundamentals, windowStart)
	})
	refreshWithRetry("M1 gap fill", func() error {
		return catchup.FillM1Gaps(ctx, gateway, candles, tracked)
	})

	catchup.RefreshTradingStatus(ctx, gateway, symbols, fundamentals)
	// Los pasos de abajo son de cadencia diaria (se recalculan tras el
	// cierre del mercado): la marca fundamental_refresh_log hace que un
	// reinicio del contenedor dentro de la MISMA ventana de mantenimiento no
	// los repita -- el done_at se graba en postgres solo al terminar OK, asi
	// un refresh fallido queda stale y se reintenta en el siguiente arranque.
	refreshFundamentalsOnce(ctx, fundamentals, "market metrics", windowStart, func() error {
		catchup.RefreshMarketMetrics(ctx, gateway, symbols, fundamentals)
		return nil
	})
	// RefreshEarningsHistory va DESPUES de RefreshMarketMetrics: este es el
	// que pisa next_earnings_date con el dato vigente de TastyTrade, asi que
	// el lote de "vencidos o nunca buscados" que queda despues es chico (solo
	// emisores cuyo earnings ya paso o que TastyTrade no cubre) -- el
	// COALESCE del upsert nunca pisa una fecha vigente con una prediccion.
	refreshFundamentalsOnce(ctx, fundamentals, "earnings history", windowStart, func() error {
		return catchup.RefreshEarningsHistory(ctx, gateway, fundamentals)
	})

	// En background: descarga+parseo del companyfacts.zip de SEC EDGAR
	// (~1.5GB, hasta 20 min la primera vez del dia) y de los ZIPs
	// trimestrales de insiders no deben demorar el arranque de la ventana
	// de mantenimiento ni bloquear la siguiente vuelta del ciclo -- mismo
	// patron que CompletableFuture.runAsync en la version Java.
	go refreshFundamentalsOnce(ctx, fundamentals, "external fundamentals", windowStart, func() error {
		return catchup.RefreshExternalFundamentals(ctx, edgar, insiders, finra, profile, symbols, fundamentals)
	})
}

// refreshFundamentalsOnce corre el refresh solo si no se completo ya en la
// ventana de mantenimiento actual -- la marca fundamental_refresh_log
// sobrevive reinicios, asi un redeploy a mitad de dia no recalcula los
// datos diarios (beta, earnings, externos) que ya se calcularon tras el
// cierre. La ventana nocturna siempre los recalcula: su done_at quedo en
// la ventana ANTERIOR (el arranque de ventana se avanza cada cierre), asi
// que la comparacion no confunde "lo de ayer" con "lo de hoy" -- un done_at
// viejo es anterior a la ventana actual y dispara el recalculo.
func refreshFundamentalsOnce(ctx context.Context, fundamentals out.FundamentalsRepository, step string, windowStart time.Time, fn func() error) {
	doneAt, done, err := fundamentals.StepDoneAt(ctx, step)
	if err != nil {
		log.Error().Err(err).Str("step", step).Msg("fundamental refresh log check failed, running anyway")
	} else if done && !doneAt.Before(windowStart) {
		log.Info().Str("step", step).Time("done_at", doneAt).Msg("fundamental refresh already done for this maintenance window, skipping")
		return
	}
	refreshWithRetry(step, func() error {
		if err := fn(); err != nil {
			return err
		}
		return fundamentals.RecordStepDone(ctx, step, time.Now())
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
