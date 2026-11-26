package main

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/catchup"
	"github.com/rs/zerolog/log"
)

const (
	liveReconcileInterval = 5 * time.Minute
	// liveReconcileMaxPerTick: reintentos por tick -- un tick no debe
	// convertirse en un segundo rollout completo (13k intentos de golpe
	// re-registran los dispatch entries del rollout en curso y lo frenan,
	// confirmado en vivo el 2026-08-18: el primer tick del reconciler dejo
	// el ciclo atascado 15+ min). Los fallidos se reparten entre ticks.
	liveReconcileMaxPerTick = 100
)

type poolHealth interface {
	CanSubscribeLive() bool
}

// StartLiveReconcileLoop resuscribe cada 5 minutos los simbolos cuyo stream
// en vivo se intento y fallo -- confirmado en vivo el 2026-08-18: con el
// limite de sesiones DxLink saturado el rollout M1 fallo para varios
// simbolos y nada los reintentaba; quedaban mudos hasta el proximo ciclo.
// Tambien resuscribe las muertes SILENCIOSAS: un resubscribe fallido del
// pool tras una reconexion deja el stream mudo sin tocar la marca del
// ingestor (LiveSubscribed reporta el estado real -- confirmado en vivo el
// 2026-08-18 con OSRH). Filtra con el mismo criterio de FilterStaleSymbols
// que el rollout nocturno -- sin esto, un simbolo sin D1 nuevo hace semanas
// (fusion/deslistado, ver stale_symbols.go) nunca llega a IsAttempted() y
// este loop lo reintentaria cada 5 minutos para siempre, deshaciendo el
// ahorro del filtro del rollout.
//
// rolloutDone distingue "el rollout de esta ventana todavia esta en curso"
// (los nunca-intentados se dejan en paz, ver shouldSkipReconcileRetry) de
// "el rollout ya termino" (ahi un nunca-intentado es una foto de
// tracked/activeTracked incompleta, no un simbolo pendiente de turno, y hay
// que reintentarlo como a cualquier otro caido -- confirmado en vivo el
// 2026-09-01: TWO, LEG, RMAX y ~200 simbolos liquidos mas quedaron mudos sin
// autosanar el resto del dia por este mismo hueco).
//
// 2026-11-17: tras un corte de internet de 5h el reconciler tardaba ~7 min
// por simbolo caido (3 intentos x 45s de canal + esperas, en serie) y
// reintentaba siempre los mismos al frente de la lista. Ahora cada tick usa
// pocos workers, un presupuesto de tiempo, backoff por simbolo y se corta
// tras fallos consecutivos (ver reconcile_tick.go).
func StartLiveReconcileLoop(ctx context.Context, ingest in.IngestCandlesService, gateway out.MarketDataGateway, pool poolHealth, symbols out.SymbolRepository, candles out.CandleRepository, rolloutDone *atomic.Bool) {
	backoff := newReconcileBackoff()
	probes := reconcileProbes{isAttempted: ingest.IsAttempted, isLive: ingest.IsLive, subscribed: gateway.LiveSubscribed}
	go func() {
		ticker := time.NewTicker(liveReconcileInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !pool.CanSubscribeLive() {
					log.Warn().Msg("live reconcile: no dxlink connection available, skipping tick")
					continue
				}
				tracked, err := symbols.Tracked(ctx)
				if err != nil {
					log.Error().Err(err).Msg("live reconcile: listing tracked symbols failed")
					continue
				}
				tracked = catchup.FilterStaleSymbols(ctx, candles, tracked, time.Now())
				tracked = liveEligible(tracked)
				candidates := selectReconcileCandidates(tracked, probes, rolloutDone.Load(), backoff, time.Now(), liveReconcileMaxPerTick)
				if len(candidates) == 0 {
					continue
				}
				outcome := runReconcileTick(ctx, candidates, startLiveOnce(ingest), backoff, time.Now)
				log.Info().Int("candidates", len(candidates)).Int("started", outcome.started).Int("failed", outcome.failed).Bool("aborted", outcome.tripped).
					Msg("live reconcile tick finished")
			}
		}
	}()
}
