package main

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
)

const liveSaveFlushInterval = 2 * time.Second

// StartLiveSaveFlushLoop guarda por lote las velas en vivo recien cerradas
// de TODOS los simbolos cada 2s, en vez de un Save() individual apenas cada
// una cierra -- con ~13k simbolos cerrando su propia vela de minuto en
// momentos distintos dentro del mismo minuto, eso eran hasta ~13k
// escrituras individuales a Postgres por minuto. Un fallo del lote entero
// no pierde nada: FlushLiveSaves ya reencola cada vela en retryBuffer, que
// RetryPendingSaves reintenta por su cuenta.
func StartLiveSaveFlushLoop(ctx context.Context, ingest in.IngestCandlesService) {
	go func() {
		ticker := time.NewTicker(liveSaveFlushInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ingest.FlushLiveSaves(ctx)
			}
		}
	}()
}
