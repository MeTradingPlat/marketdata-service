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
// Los fundamentales que son REST puro (trading status, market metrics,
// earnings history, y en background el externo de SEC/FINRA) van ANTES de
// D1/H1/M1, no despues -- no compiten por conexiones DxLink con las fases
// de velas y no hay motivo para que esperen 20-30 min a que el barrido
// termine. Solo beta (D1) y prevClose/prevPostMarketVolume (M1) quedan
// despues de su fase respectiva, porque esos si dependen de velas propias
// recien sembradas (ver el cuerpo de runUniverseCycle).
func StartUniverseCycle(ctx context.Context, cfg *configs.Config, gateway out.MarketDataGateway, symbols out.SymbolRepository, candles out.CandleRepository, fundamentals out.FundamentalsRepository, volumeProfiles out.VolumeProfileRepository, ingest in.IngestCandlesService, edgar out.SharesOutstandingGateway, insiders out.InsiderOwnershipGateway, finra out.ShortInterestGateway, profile out.ProfileSharesGateway, backfilling *atomic.Bool, tracker *intraday.SnapshotTracker, fundamentalsCache *fundamentals2.FundamentalsCache, symbolsCache *metadata.SymbolsCache, liveRolloutDone *atomic.Bool, packer channelPacker) {
	go func() {
		runUniverseCycle(ctx, cfg, gateway, symbols, candles, fundamentals, volumeProfiles, ingest, edgar, insiders, finra, profile, backfilling, tracker, fundamentalsCache, symbolsCache, liveRolloutDone, true, packer)
		for {
			wait := time.Until(catchup.NextMaintenanceWindowAt(time.Now()))
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
				runUniverseCycle(ctx, cfg, gateway, symbols, candles, fundamentals, volumeProfiles, ingest, edgar, insiders, finra, profile, backfilling, tracker, fundamentalsCache, symbolsCache, liveRolloutDone, false, packer)
			}
		}
	}()
}

func runUniverseCycle(ctx context.Context, cfg *configs.Config, gateway out.MarketDataGateway, symbols out.SymbolRepository, candles out.CandleRepository, fundamentals out.FundamentalsRepository, volumeProfiles out.VolumeProfileRepository, ingest in.IngestCandlesService, edgar out.SharesOutstandingGateway, insiders out.InsiderOwnershipGateway, finra out.ShortInterestGateway, profile out.ProfileSharesGateway, backfilling *atomic.Bool, tracker *intraday.SnapshotTracker, fundamentalsCache *fundamentals2.FundamentalsCache, symbolsCache *metadata.SymbolsCache, liveRolloutDone *atomic.Bool, firstRun bool, packer channelPacker) {
	// Pipeline del backfill: D1 primero, se cierran las conexiones, se calcula
	// beta (D1), luego H1 (se cierra, backfill nativo propio), y por ultimo
	// M1 que se queda suscrito. backfilling bloquea SOLO las rutas de
	// signal-processing-service mientras dura (ver router.go/BackfillGate).
	// Las rutas del frontend NO se bloquean.
	backfilling.Store(true)
	defer backfilling.Store(false)

	tracked := catchup.ReconcileAndTracked(ctx, gateway, symbols)
	if len(tracked) == 0 {
		log.Error().Msg("universe sweep returned no symbols, skipping live M1 rollout")
		return
	}
	// Justo despues de reconciliar (unico punto que escribe Upsert/Deactivate
	// en tracked_symbols) -- asi SymbolsCache siempre refleja el universo
	// recien reconciliado antes de que arranque el barrido.
	symbolsCache.ReloadAll(ctx)
	// fundamentalsCache tambien se carga ACA, antes de que arranque el
	// barrido -- sin este reload temprano, un arranque en frio (proceso
	// recien iniciado, cache vacio) serviria fundamentales vacios/en cero
	// durante TODO el ciclo (hasta 20+ min), en vez de los ultimos datos
	// buenos que ya estan en Postgres de la ventana anterior. El reload de
	// mas abajo (al terminar el ciclo) sigue refrescandolo con lo nuevo.
	fundamentalsCache.ReloadAll(ctx)
	// Mismo motivo para el SnapshotTracker: sin este seed temprano, Search() rankearia por el volumen de AYER hasta el sweep M1.
	seedSnapshotTracker(ctx, candles, tracker, tracked)

	windowStart := catchup.LastMaintenanceWindowStart(time.Now())

	// Fundamentales que NO dependen de velas propias (REST puro a
	// TastyTrade/SEC/FINRA) van ANTES del barrido de velas, no despues --
	// pedido explicito del usuario: un redeploy no deberia dejar
	// market-cap/beta-del-proveedor/earnings/dividendos desactualizados
	// durante los 20-30 min que tarda D1+M1, cuando nada de esto necesita
	// esperar a esas fases. Solo quedan DESPUES del barrido los que si
	// dependen de velas propias: RefreshBeta (D1) y RefreshPrevClose/
	// RefreshPrevPostMarketVolume (M1), ver mas abajo.
	if last := lastTradingStatusAtUnix.Load(); time.Since(time.Unix(last, 0)) > 10*time.Minute {
		catchup.RefreshTradingStatus(ctx, gateway, symbols, fundamentals, fundamentalsCache)
		lastTradingStatusAtUnix.Store(time.Now().Unix())
	}
	refreshFundamentalsOnce(ctx, fundamentals, "market metrics", windowStart, func() error {
		catchup.RefreshMarketMetrics(ctx, gateway, symbols, fundamentals, fundamentalsCache)
		return nil
	})
	// RefreshEarningsHistory va DESPUES de RefreshMarketMetrics: este es el
	// que pisa next_earnings_date con el dato vigente de TastyTrade, asi que
	// el lote de "vencidos o nunca buscados" que queda despues es chico (solo
	// emisores cuyo earnings ya paso o que TastyTrade no cubre) -- el
	// COALESCE del upsert nunca pisa una fecha vigente con una prediccion.
	refreshFundamentalsOnce(ctx, fundamentals, "earnings history", windowStart, func() error {
		return catchup.RefreshEarningsHistory(ctx, gateway, fundamentals, fundamentalsCache)
	})
	// El cache ya quedo al dia con lo de arriba (MergeMarketMetrics/
	// MergeEarningsHistory corren DENTRO de cada Refresh*, ver
	// fundamentals_cache.go) -- no hace falta un ReloadAll aca.

	// En background: descarga+parseo del companyfacts.zip de SEC EDGAR
	// (~1.5GB, hasta 20 min la primera vez del dia) y de los ZIPs
	// trimestrales de insiders no deben demorar el arranque del barrido de
	// velas ni bloquear la siguiente vuelta del ciclo. Mismo motivo que
	// arriba: MergeExternalFundamentals ya deja el cache al dia, sin
	// esperar a este goroutine para releer todo de Postgres.
	go func() {
		refreshFundamentalsOnce(ctx, fundamentals, "external fundamentals", windowStart, func() error {
			return catchup.RefreshExternalFundamentals(ctx, edgar, insiders, finra, profile, symbols, fundamentals, fundamentalsCache)
		})
	}()

	// Open interest de opciones via TastyTrade (por simbolo, sin batch): se
	// corre en background para no demorar las fases de velas y mantiene
	// el cache al dia para responder GetSymbolDetails en 0ms.
	go func() {
		refreshFundamentalsOnce(ctx, fundamentals, "open interest", windowStart, func() error {
			return catchup.RefreshOpenInterest(ctx, gateway.(out.OpenInterestGateway), fundamentals, fundamentalsCache, windowStart)
		})
	}()

	if !firstRun {
		gateway.ResetLiveConnections()
	}

	// En falso durante TODO el rollout de esta ventana -- ver
	// shouldSkipReconcileRetry: mientras este en falso, el reconciler deja
	// en paz a los simbolos nunca intentados en vez de pelear con este mismo
	// rollout por su turno.
	liveRolloutDone.Store(false)

	// FASE 1: D1 + beta (guard por-simbolo, se calcula con D1 propio).
	pauseCompression(ctx, candles)
	catchup.RunSweepPhase(ctx, gateway, candles, ingest, tracked, domain.D1, cfg.SweepWorkers)
	// RefreshBeta usa el guard por-simbolo beta_updated_at: solo calcula
	// los simbolos cuyo beta no se calculo en esta ventana de
	// mantenimiento (ver beta_refresh.go).
	refreshWithRetry("beta", func() error {
		return catchup.RefreshBeta(ctx, candles, fundamentals, fundamentalsCache, windowStart)
	})

	// Simbolos sin D1 nuevo hace demasiado (fusion de SPAC, deslistado, nota
	// vencida -- TastyTrade los sigue listando "activos" pero dxFeed no manda
	// mas dato) no pagan H1/M1/suscripcion en vivo -- el D1 de ARRIBA, que
	// SIEMPRE corre para el universo completo, es la unica señal de "¿ya
	// volvio?" que hace falta (ver FilterStaleSymbols).
	activeTracked := catchup.FilterStaleSymbols(ctx, candles, tracked, time.Now())

	// FASE 2: H1, desde cero sesiones (RunSweepPhase cierra al terminar).
	catchup.RunSweepPhase(ctx, gateway, candles, ingest, activeTracked, domain.H1, cfg.SweepWorkers)

	// FASE 3: M1 en vivo (se queda suscrito) + prevClose (va DESPUES del rollout M1).
	catchup.RunSweepPhase(ctx, gateway, candles, ingest, activeTracked, domain.M1, cfg.SweepWorkers)
	resumeCompression(candles)

	// Sembrar el SnapshotTracker con UNA sola consulta de lote justo
	// despues del sweep M1 y antes de abrir las suscripciones en vivo.
	seedSnapshotTracker(ctx, candles, tracker, activeTracked)

	select {
	case <-time.After(liveRolloutSessionGrace):
	case <-ctx.Done():
		return
	}

	eligibleLive := liveEligible(activeTracked)
	log.Info().Int("eligible", len(eligibleLive)).Int("excluded", len(activeTracked)-len(eligibleLive)).Msg("live rollout symbols eligibility")
	packer.SetMaxChannelsPerConn(cfg.LiveMaxChannelsPerConn)
	startLiveUniverse(ctx, ingest, eligibleLive)
	// A partir de aca un simbolo que siga sin IsAttempted no esta "esperando
	// su turno" -- se cayo de la foto de tracked/activeTracked y el reconciler ya lo puede tratar como
	// cualquier otro caido.
	liveRolloutDone.Store(true)
	refreshWithRetry("prev close", func() error {
		return catchup.RefreshPrevClose(ctx, candles, fundamentals, fundamentalsCache, windowStart)
	})
	refreshWithRetry("prev post market volume", func() error {
		return catchup.RefreshPrevPostMarketVolume(ctx, candles, fundamentals, fundamentalsCache, windowStart)
	})
	refreshWithRetry("volume profile", func() error {
		return catchup.RefreshVolumeProfile(ctx, volumeProfiles, windowStart, time.Now())
	})
	// Todo lo que este ciclo escribio ya quedo reflejado en el cache al
	// momento de escribirse. Ya no hace falta un ReloadAll de cierre.
}
