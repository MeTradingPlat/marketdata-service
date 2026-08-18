package main

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/catchup"
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
func StartTradingStatusLoop(ctx context.Context, gateway out.MarketDataGateway, symbols out.SymbolRepository, fundamentals out.FundamentalsRepository) {
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
				catchup.RefreshTradingStatus(ctx, gateway, symbols, fundamentals)
				lastTradingStatusAtUnix.Store(time.Now().Unix())
			}
		}
	}()
}

func isMarketActiveWindow(now time.Time) bool {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return true
	}
	nowET := now.In(loc)
	if nowET.Weekday() == time.Saturday || nowET.Weekday() == time.Sunday {
		return false
	}
	hour := nowET.Hour()
	return hour >= 4 && hour < 20
}
