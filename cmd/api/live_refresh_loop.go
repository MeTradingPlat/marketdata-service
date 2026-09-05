package main

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

// liveRefreshInterval: 1 minuto -- el usuario no quiere quedarse mas de un
// minuto sin saber si una suscripcion sigue viva de verdad (ver el
// comentario de CandlePool.RefreshLiveSubscriptions sobre por que un
// re-Add repetido es seguro).
const liveRefreshInterval = 1 * time.Minute

// StartLiveRefreshLoop reenvia el Add de cada suscripcion M1 viva cada
// minuto, salvo mientras el fill/refill esta en curso (backfilling=true) --
// en ese momento el sweep ya esta pidiendo y guardando velas para el
// universo entero, sumarle un refresco de las suscripciones en vivo
// encima solo compite por los mismos canales sin necesidad.
func StartLiveRefreshLoop(ctx context.Context, gateway out.MarketDataGateway, backfilling *atomic.Bool) {
	go func() {
		ticker := time.NewTicker(liveRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if backfilling.Load() {
					continue
				}
				log.Debug().Msg("live refresh: resending live subscription for every channel")
				gateway.RefreshLiveSubscriptions(ctx)
			}
		}
	}()
}
