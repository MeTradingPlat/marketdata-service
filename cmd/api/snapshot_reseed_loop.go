package main

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/intraday"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/metadata"
	"github.com/rs/zerolog/log"
)

// snapshotReconcileInterval: el unico Seed() de SnapshotTracker corre una
// vez al dia, al cierre (ver NextMaintenanceWindowAt) -- desde la
// medianoche ET hasta esa ventana, el tracker depende SOLO de
// RecordClosedCandle via el stream en vivo. Un simbolo cuyos primeros
// ticks del dia llegan por un camino que no pasa por ahi (backfill/catchup,
// CandleRepository.Save directo -- ver el comentario de Seed) se queda con
// el volumen de AYER para el ranking de Activos (rankByTodayVolume)
// durante todo ese hueco. Confirmado en vivo el 2026-09-08: DVLT (1000) por
// encima de TSLL (25341) y NVDA (18171) en la lista, sin moverse en varios
// minutos pese a que el volumen real seguia creciendo.
const snapshotReconcileInterval = 20 * time.Minute

// StartSnapshotReconcileLoop corrige el tracker contra la BD cada
// snapshotReconcileInterval via MergeReconcile (nunca Seed: ver por que en
// su comentario) -- asi un simbolo que goteo su primer volumen del dia por
// un camino que no paso por RecordClosedCandle deja de mostrar volumen de
// ayer en el ranking apenas esa BD lo refleje, en vez de esperar a la
// ventana de mantenimiento de esta noche. Se salta mientras backfilling=true
// por el mismo motivo que StartLiveRefreshLoop: el sweep nocturno ya satura
// la misma BD con su propio barrido.
func StartSnapshotReconcileLoop(ctx context.Context, candles out.CandleRepository, tracker *intraday.SnapshotTracker, symbolsCache *metadata.SymbolsCache, backfilling *atomic.Bool) {
	go func() {
		// Sin esta corrida inmediata, un despliegue en horario de mercado
		// arranca ciego durante los primeros snapshotReconcileInterval
		// minutos -- exactamente el hueco que este mismo mecanismo existe
		// para cerrar (confirmado en vivo el 2026-09-08, justo tras
		// desplegar este archivo). No corre si el arranque cayo en medio
		// del barrido nocturno (backfilling=true): ese barrido ya va a
		// sembrar el tracker el solo al terminar (ver seedSnapshotTracker).
		if !backfilling.Load() {
			reconcileSnapshotTracker(ctx, candles, tracker, symbolsCache)
		}

		ticker := time.NewTicker(snapshotReconcileInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if backfilling.Load() {
					continue
				}
				reconcileSnapshotTracker(ctx, candles, tracker, symbolsCache)
			}
		}
	}()
}

func reconcileSnapshotTracker(ctx context.Context, candles out.CandleRepository, tracker *intraday.SnapshotTracker, symbolsCache *metadata.SymbolsCache) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		log.Error().Err(err).Msg("snapshot reconcile: loading America/New_York failed")
		return
	}
	nowET := time.Now().In(loc)
	day := time.Date(nowET.Year(), nowET.Month(), nowET.Day(), 0, 0, 0, 0, loc)

	tracked, err := symbolsCache.Tracked(ctx)
	if err != nil {
		log.Error().Err(err).Msg("snapshot reconcile: listing tracked symbols failed")
		return
	}
	symbols := make([]string, len(tracked))
	for i, s := range tracked {
		symbols[i] = s.Symbol
	}

	start := time.Now()
	snapshots, err := candles.GetIntradaySessionsBatch(ctx, symbols)
	if err != nil {
		log.Error().Err(err).Msg("snapshot reconcile: batch query failed")
		return
	}
	tracker.MergeReconcile(day, snapshots)
	log.Info().Int("symbols", len(snapshots)).Dur("elapsed", time.Since(start)).Msg("snapshot tracker reconciled")
}
