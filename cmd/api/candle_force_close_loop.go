package main

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/external/tastytrade"
)

// candleForceCloseInterval: 1s -- el mismo orden de magnitud que el margen
// de gracia de CloseElapsedForming (closeElapsedGracePeriod), para que un
// simbolo silencioso no espere de mas ENCIMA del margen ya dado al tick
// organico.
const candleForceCloseInterval = 1 * time.Second

// StartCandleForceCloseLoop cierra por reloj, cada segundo, cualquier vela
// M1 en formacion cuyo minuto ya paso -- sin esto, un simbolo sin ticks
// justo en el cierre (silencio real de mercado) se queda "en formacion"
// hasta el proximo trade, que en un simbolo poco liquido puede tardar
// minutos u horas. Ver CandlePool.CloseElapsedForming.
func StartCandleForceCloseLoop(ctx context.Context, pool *tastytrade.CandlePool) {
	go func() {
		ticker := time.NewTicker(candleForceCloseInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pool.CloseElapsedForming(time.Now())
			}
		}
	}()
}
