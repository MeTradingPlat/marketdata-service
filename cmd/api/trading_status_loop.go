package main

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/fundamentals"
)

const tradingStatusInterval = 15 * time.Minute

// lastTradingStatusAtUnix: la ultima corrida del trading status (loop o
// ciclo) -- el ciclo salta el suyo si el loop corrio hace poco (ver
// universe_cycle.go): el dato de halt es el mismo y el ciclo no debe pagar
// 100s de REST por refrescarlo dos veces.
var lastTradingStatusAtUnix atomic.Int64

// StartTradingStatusLoop refresca halt/trading-status cada 15 minutos
// mientras el mercado puede estar activo (pre-market a post-market
// extendido, 4am-8pm ET, dias habiles) -- es el unico dato de fundamentales
// que de verdad pierde sentido si se entera recien la manana siguiente (ver
// RefreshTradingStatus). Fuera de esa ventana no hay nada que descubrir, no
// vale la pena golpear la API por las dudas.
func StartTradingStatusLoop(ctx context.Context, gateway out.MarketDataGateway, symbols out.SymbolRepository, fundamentalsRepo out.FundamentalsRepository, fundamentalsCache *fundamentals.FundamentalsCache) {
	go func() {
		ticker := time.NewTicker(tradingStatusInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !isMarketActiveWindow(time.Now()) {
					continue
				}
				refreshTradingStatusStep(ctx, gateway, symbols, fundamentalsRepo, fundamentalsCache)
			}
		}
	}()
}
