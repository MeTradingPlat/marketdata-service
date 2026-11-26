package main

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/catchup"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/fundamentals"
)

func refreshTradingStatusStep(ctx context.Context, gateway out.MarketDataGateway, symbols out.SymbolRepository, fundamentalsRepo out.FundamentalsRepository, fundamentalsCache *fundamentals.FundamentalsCache) {
	// El cache se actualiza DENTRO de RefreshTradingStatus (ver
	// MergeDividends) apenas Postgres confirma -- un HALT nuevo se
	// ve en /marketdata/fundamentals/realtime al instante, no hay
	// que esperar ni un ReloadAll aparte.
	if err := catchup.RefreshTradingStatus(ctx, gateway, symbols, fundamentalsRepo, fundamentalsCache); err == nil {
		lastTradingStatusAtUnix.Store(time.Now().Unix())
	}
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
