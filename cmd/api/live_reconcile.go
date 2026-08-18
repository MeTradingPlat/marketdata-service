package main

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

const liveReconcileInterval = 5 * time.Minute

// StartLiveReconcileLoop resuscribe cada 5 minutos los simbolos cuyo stream
// en vivo no arranco -- confirmado en vivo el 2026-08-18: con el limite de
// sesiones DxLink saturado (sesiones huerfanas de contenedores muertos) el
// rollout M1 fallo para 6 simbolos y nada los reintentaba despues; quedaron
// mudos hasta el proximo ciclo. Los streams que mueren DESPUES de arrancar
// los resuscribe el pool al reconectar (handleConnectionReconnect) -- este
// loop cubre los que nunca llegaron a estar vivos, y de paso adopta
// simbolos nuevos que aparezcan en el universo.
func StartLiveReconcileLoop(ctx context.Context, ingest in.IngestCandlesService, symbols out.SymbolRepository) {
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
				for _, s := range tracked {
					if ingest.IsLive(s.Symbol) {
						continue
					}
					startLiveWithRetry(ctx, ingest, s.Symbol)
				}
			}
		}
	}()
}
