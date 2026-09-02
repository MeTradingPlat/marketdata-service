package main

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
)

// StartSaveRetryLoop reintenta las velas en vivo que fallaron al guardarse
// (Postgres caido un momento, ver ingestion.saveRetryBuffer) -- una caida
// breve como la de esta noche (segundos) se resuelve en el proximo tick,
// sin esperar a la ventana de mantenimiento siguiente. El intervalo hace
// backoff en fallos consecutivos y vuelve al piso apenas uno se resuelve
// sin error (ver nextSaveRetryInterval).
func StartSaveRetryLoop(ctx context.Context, ingest in.IngestCandlesService) {
	go func() {
		interval := saveRetryBaseInterval
		timer := time.NewTimer(interval)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				interval = nextSaveRetryInterval(interval, ingest.RetryPendingSaves(ctx))
				timer.Reset(interval)
			}
		}
	}()
}
