package main

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
)

// liveSaveFlushInterval alineado a la cadencia real de M1 (una vela por
// simbolo por minuto) -- confirmado en vivo el 2026-09-03: con 2s se hacian
// hasta 30 commits/escrituras por minuto para el mismo volumen total de
// datos que un solo flush por minuto ya cubre, y cada commit fuerza un
// fsync. Con el disco del VAIO saturado (checkpoints de Postgres tardando
// 200-300s ese mismo dia), 30 fsyncs/min de mas era presion evitable sobre
// el mismo disco. No afecta el dato en vivo que ve el usuario -- recentCache,
// SnapshotTracker y el broadcast WS ya se actualizan sincronicamente en el
// callback de StreamLive, sin depender de este flush; lo unico que se demora
// es cuando queda escrito en Postgres, y el barrido nocturno cubre cualquier
// hueco por watermark si el proceso se reinicia antes del proximo flush.
const liveSaveFlushInterval = 60 * time.Second

// StartLiveSaveFlushLoop guarda por lote las velas en vivo recien cerradas
// de TODOS los simbolos una vez por minuto, en vez de un Save() individual
// apenas cada una cierra -- con ~13k simbolos cerrando su propia vela de
// minuto en momentos distintos dentro del mismo minuto, eso eran hasta ~13k
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
