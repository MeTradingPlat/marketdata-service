package main

import (
	"context"
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

// StartLiveReconcileLoop resuscribe cada 5 minutos los simbolos cuyo stream
// en vivo se intento y fallo -- confirmado en vivo el 2026-08-18: con el
// limite de sesiones DxLink saturado el rollout M1 fallo para varios
// simbolos y nada los reintentaba; quedaban mudos hasta el proximo ciclo.
// Solo toca los INTENTADOS y fallidos (IsAttempted): el primer tick cae
// cuando el ciclo recien va por D1/beta y reintentar los nunca-intentados
// pelea con el rollout en curso. Tambien resuscribe las muertes SILENCIOSAS:
// un resubscribe fallido del pool tras una reconexion deja el stream mudo
// sin tocar la marca del ingestor (LiveSubscribed reporta el estado real --
// confirmado en vivo el 2026-08-18 con OSRH). Filtra con el mismo criterio
// de FilterStaleSymbols que el rollout nocturno -- sin esto, un simbolo sin
// D1 nuevo hace semanas (fusion/deslistado, ver stale_symbols.go) nunca
// llega a IsAttempted() y este loop lo reintentaria cada 5 minutos para
// siempre, deshaciendo el ahorro del filtro del rollout.
func StartLiveReconcileLoop(ctx context.Context, ingest in.IngestCandlesService, gateway out.MarketDataGateway, symbols out.SymbolRepository, candles out.CandleRepository) {
	go func() {
		ticker := time.NewTicker(liveReconcileInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				tracked, err := symbols.Tracked(ctx)
				if err != nil {
					log.Error().Err(err).Msg("live reconcile: listing tracked symbols failed")
					continue
				}
				tracked = catchup.FilterStaleSymbols(ctx, candles, tracked, time.Now())
				retried := 0
				for _, s := range tracked {
					if !ingest.IsAttempted(s.Symbol) || ingest.IsLive(s.Symbol) && gateway.LiveSubscribed(s.Symbol) {
						continue
					}
					if retried >= liveReconcileMaxPerTick {
						break
					}
					retried++
					startLiveWithRetry(ctx, ingest, s.Symbol)
				}
			}
		}
	}()
}
