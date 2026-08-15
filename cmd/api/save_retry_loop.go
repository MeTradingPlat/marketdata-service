package main

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
)

const saveRetryInterval = 15 * time.Second

// StartSaveRetryLoop reintenta cada 15s las velas en vivo que fallaron al
// guardarse (Postgres caido un momento, ver ingestion.saveRetryBuffer) --
// una caida breve como la de esta noche (segundos) se resuelve en el
// proximo tick, sin esperar a la ventana de mantenimiento siguiente.
func StartSaveRetryLoop(ctx context.Context, ingest in.IngestCandlesService) {
	go func() {
		ticker := time.NewTicker(saveRetryInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ingest.RetryPendingSaves(ctx)
			}
		}
	}()
}
