package main

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/catchup"
	fundamentals2 "github.com/MeTradingPlat/marketdata-service/internal/core/service/fundamentals"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/intraday"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/metadata"
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
func StartUniverseCycle(ctx context.Context, cfg *configs.Config, gateway out.MarketDataGateway, symbols out.SymbolRepository, candles out.CandleRepository, fundamentals out.FundamentalsRepository, ingest in.IngestCandlesService, edgar out.SharesOutstandingGateway, insiders out.InsiderOwnershipGateway, finra out.ShortInterestGateway, profile out.ProfileSharesGateway, backfilling *atomic.Bool, tracker *intraday.SnapshotTracker, fundamentalsCache *fundamentals2.FundamentalsCache, symbolsCache *metadata.SymbolsCache, liveRolloutDone *atomic.Bool) {
	go func() {
		runUniverseCycle(ctx, cfg, gateway, symbols, candles, fundamentals, ingest, edgar, insiders, finra, profile, backfilling, tracker, fundamentalsCache, symbolsCache, liveRolloutDone, true)
		for {
			wait := time.Until(catchup.NextMaintenanceWindowAt(time.Now()))
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
				runUniverseCycle(ctx, cfg, gateway, symbols, candles, fundamentals, ingest, edgar, insiders, finra, profile, backfilling, tracker, fundamentalsCache, symbolsCache, liveRolloutDone, false)
			}
		}
	}()
}

func runUniverseCycle(ctx context.Context, cfg *configs.Config, gateway out.MarketDataGateway, symbols out.SymbolRepository, candles out.CandleRepository, fundamentals out.FundamentalsRepository, ingest in.IngestCandlesService, edgar out.SharesOutstandingGateway, insiders out.InsiderOwnershipGateway, finra out.ShortInterestGateway, profile out.ProfileSharesGateway, backfilling *atomic.Bool, tracker *intraday.SnapshotTracker, fundamentalsCache *fundamentals2.FundamentalsCache, symbolsCache *metadata.SymbolsCache, liveRolloutDone *atomic.Bool, firstRun bool) {
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
	// Justo despues de reconciliar (unico punto que escribe Upsert/Deactivate
	// en tracked_symbols) -- asi SymbolsCache siempre refleja el universo
	// recien reconciliado antes de que el gate se abra a peticiones externas.
	symbolsCache.ReloadAll(ctx)

	// El gate solo tiene sentido cuando el barrido corre en la ventana de
	// mantenimiento real (mercado cerrado) -- ahi bloquear no cuesta nada,
	// nadie esta escaneando. Pero firstRun corre en CADA arranque del
	// proceso, sin importar la hora: un redeploy a mitad de la sesion
	// (confirmado en vivo el 2026-08-20, varios seguidos) disparaba el
	// mismo sweep completo D1+H1+M1+fundamentales CON el gate puesto,
	// dejando a signal-processing-service reintentando "en mantenimiento"
	// varios minutos en pleno horario de mercado -- exactamente la clase de
	// atraso en senales que se estaba investigando. La mayoria de los datos
	// ya existen de antes del reinicio (Backfill/StreamLive retoman desde
	// watermark, no repiten todo), asi que el riesgo real de "vela a medio
	// rellenar" en un firstRun en horario de mercado es bajo comparado con
	// bloquear toda la plataforma.
	gate := !firstRun || !isMarketActive(time.Now())
	if gate {
		backfilling.Store(true)
		defer backfilling.Store(false)
	}

	if !firstRun {
		gateway.ResetLiveConnections()
	}

	// En falso durante TODO el rollout de esta ventana -- ver
	// shouldSkipReconcileRetry: mientras este en falso, el reconciler deja
	// en paz a los simbolos nunca intentados en vez de pelear con este mismo
	// rollout por su turno.
	liveRolloutDone.Store(false)

	// FASE 1: D1 + beta (guard por-simbolo, se calcula con D1 propio).
	catchup.RunSweepPhase(ctx, gateway, candles, ingest, tracked, domain.D1, cfg.SweepWorkers)
	windowStart := catchup.LastMaintenanceWindowStart(time.Now())
	// RefreshBeta usa el guard por-simbolo beta_updated_at: solo calcula
	// los simbolos cuyo beta no se calculo en esta ventana de
	// mantenimiento (ver beta_refresh.go).
	refreshWithRetry("beta", func() error {
		return catchup.RefreshBeta(ctx, candles, fundamentals, windowStart)
	})

	// Simbolos sin D1 nuevo hace demasiado (fusion de SPAC, deslistado, nota
	// vencida -- TastyTrade los sigue listando "activos" pero dxFeed no manda
	// mas dato) no pagan H1/M1/suscripcion en vivo -- el D1 de ARRIBA, que
	// SIEMPRE corre para el universo completo, es la unica señal de "¿ya
	// volvio?" que hace falta (ver FilterStaleSymbols).
	activeTracked := catchup.FilterStaleSymbols(ctx, candles, tracked, time.Now())

	// FASE 2: H1, desde cero sesiones (RunSweepPhase cierra al terminar).
	catchup.RunSweepPhase(ctx, gateway, candles, ingest, activeTracked, domain.H1, cfg.SweepWorkers)

	// FASE 3: M1 en vivo (se queda suscrito) + prevClose (se calcula desde
	// las velas M1 de la sesion anterior, asi que va DESPUES del rollout
	// M1 -- corria antes con la tabla M1 vacia en un refill en frio y
	// calculaba 0). Sin verificacion de huecos de 10 dias: cada vela
	// guardada (en vivo y en refill) actualiza su watermark, asi que un
	// reinicio retoma desde el ultimo minuto guardado y el replay de la
	// suscripcion rellena solo el hueco de las horas caidas -- pedir 10
	// dias era redundante y costoso (una pasada de 25+ min sobre el M1
	// completo en el primer refill).
	// El sweep M1 es el REFILL que avanza el watermark M1 -- sin el, el
	// rollout (StreamLive) guardaba el replay con withWatermark=false y el
	// watermark nunca avanzaba: cada ciclo re-jugaba ~1.5 dias de M1 por
	// simbolo (rollout de 11+ min, confirmado en vivo el 2026-08-18). Con el
	// sweep, el watermark avanza diario y el rollout solo re-juega el hueco
	// del downtime (~minutos).
	catchup.RunSweepPhase(ctx, gateway, candles, ingest, activeTracked, domain.M1, cfg.SweepWorkers)

	// Sembrar el SnapshotTracker con UNA sola consulta de lote (el mismo
	// costo que antes pagaba CADA request de fundamentals/realtime) justo
	// despues del sweep M1 y antes de abrir las suscripciones en vivo -- sin
	// esto, GetSnapshotsBatch caeria al fallback de BD para el universo
	// entero hasta que cada simbolo recibiera su primer tick en vivo.
	seedSnapshotTracker(ctx, candles, tracker, activeTracked)

	startLiveUniverse(ctx, ingest, activeTracked)
	// A partir de aca un simbolo que siga sin IsAttempted no esta "esperando
	// su turno" -- se cayo de la foto de tracked/activeTracked (ver
	// seedRetryDelay mas abajo para el precedente de esa consulta fallando
	// en silencio bajo presion) y el reconciler ya lo puede tratar como
	// cualquier otro caido.
	liveRolloutDone.Store(true)
	refreshWithRetry("prev close", func() error {
		return catchup.RefreshPrevClose(ctx, candles, fundamentals, windowStart)
	})

	// El trading status lo refresca el loop cada 15 min mientras el mercado
	// esta activo -- si corrio hace menos de 10 min, el ciclo salta el suyo
	// (ahorra ~100s de REST por ciclo, ver trading_status_loop.go).
	if last := lastTradingStatusAtUnix.Load(); time.Since(time.Unix(last, 0)) > 10*time.Minute {
		catchup.RefreshTradingStatus(ctx, gateway, symbols, fundamentals)
		lastTradingStatusAtUnix.Store(time.Now().Unix())
	}
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

	// Recien aca terminaron TODOS los pasos sincronos que escriben
	// fundamentales (beta, market metrics, earnings, prevClose, trading
	// status) -- refrescar el cache de una sola vez aca, en vez de parchear
	// campo por campo en cada Upsert, deja listo en memoria exactamente lo
	// mismo que ya se abre a peticiones externas (backfilling.Store(false)
	// via el defer de mas arriba corre justo despues de este return).
	fundamentalsCache.ReloadAll(ctx)

	// En background: descarga+parseo del companyfacts.zip de SEC EDGAR
	// (~1.5GB, hasta 20 min la primera vez del dia) y de los ZIPs
	// trimestrales de insiders no deben demorar el arranque de la ventana
	// de mantenimiento ni bloquear la siguiente vuelta del ciclo -- mismo
	// patron que CompletableFuture.runAsync en la version Java.
	go func() {
		refreshFundamentalsOnce(ctx, fundamentals, "external fundamentals", windowStart, func() error {
			return catchup.RefreshExternalFundamentals(ctx, edgar, insiders, finra, profile, symbols, fundamentals)
		})
		// sharesOutstanding/floatShares/shortInterest recien quedan
		// disponibles cuando esto termina (hasta 20 min despues de abierto
		// el gate) -- un segundo reload los recoge sin esperar a la
		// proxima ventana de mantenimiento.
		fundamentalsCache.ReloadAll(ctx)
	}()
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

// seedRetryDelay le da tiempo a Postgres a soltar la presion de escritura
// del sweep M1 (confirmado en vivo el 2026-08-26: la consulta de lote de
// ~13k simbolos corrio justo en medio de "out of shared memory" del sweep y
// devolvio filas para solo 11590/13222 -- sin error, silenciosamente
// incompleta. Repetir la MISMA consulta minutos despues, ya sin esa presion,
// encontro los datos completos y correctos para los simbolos que habian
// faltado). No hay forma barata de distinguir "de verdad no opero hoy" de
// "la consulta lo perdio por presion" solo con el conteo, asi que se
// reintenta sin condicion cuando falta alguno -- si de verdad no opero, la
// segunda vuelta tampoco trae nada y no hace daño.
const seedRetryDelay = 30 * time.Second

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

	if missing := missingSymbols(symbols, snapshots); len(missing) > 0 {
		log.Warn().Int("missing", len(missing)).Int("total", len(symbols)).
			Msg("snapshot tracker seed came back incomplete, retrying missing symbols")
		select {
		case <-time.After(seedRetryDelay):
		case <-ctx.Done():
			tracker.Seed(day, snapshots)
			return
		}
		if retried, retryErr := candles.GetIntradaySessionsBatch(ctx, missing); retryErr == nil {
			for symbol, snap := range retried {
				snapshots[symbol] = snap
			}
			log.Info().Int("recovered", len(retried)).Msg("snapshot tracker seed retry finished")
		} else {
			log.Error().Err(retryErr).Msg("snapshot tracker seed retry failed")
		}
	}

	tracker.Seed(day, snapshots)
	log.Info().Int("symbols", len(snapshots)).Dur("elapsed", time.Since(start)).Msg("snapshot tracker seeded")

	// SeedLastClose por separado: sin esto, el fallback de precio actual
	// (GetSnapshotsBatch, ver LastClose) no tendria nada que devolver para
	// ningun simbolo hasta su primer tick en vivo, y caeria de nuevo en la
	// consulta lenta para el universo ENTERO justo tras este mismo
	// despliegue -- confirmado en vivo el 2026-08-20.
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
	tracker.SeedLastClose(lastClose)
	log.Info().Int("symbols", len(lastClose)).Dur("elapsed", time.Since(lastStart)).Msg("last-close tracker seeded")
}

func missingSymbols(symbols []string, snapshots map[string]domain.IntradaySnapshot) []string {
	missing := make([]string, 0, len(symbols)-len(snapshots))
	for _, symbol := range symbols {
		if _, ok := snapshots[symbol]; !ok {
			missing = append(missing, symbol)
		}
	}
	return missing
}

// isMarketActive: mismo rango que effective_start/effective_end en
// signal-processing-service (04:00-20:00 ET, pre/post-market extendido
// incluido) -- no chequea feriados/fin de semana a proposito, un firstRun
// que cae en uno de esos dias simplemente vuelve a la conducta segura de
// siempre (gate puesto) en vez de arriesgar bloquear trafico real por un
// falso negativo.
func isMarketActive(now time.Time) bool {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return true
	}
	nowET := now.In(loc)
	if nowET.Weekday() == time.Saturday || nowET.Weekday() == time.Sunday {
		return false
	}
	open := time.Date(nowET.Year(), nowET.Month(), nowET.Day(), 4, 0, 0, 0, loc)
	closeTime := time.Date(nowET.Year(), nowET.Month(), nowET.Day(), 20, 0, 0, 0, loc)
	return !nowET.Before(open) && nowET.Before(closeTime)
}
