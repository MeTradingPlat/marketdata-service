package injector

import (
	"context"
	"fmt"
	"strconv"
	"sync/atomic"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/incoming/handler"
	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/external/finra"
	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/external/secedgar"
	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/external/tastytrade"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/discovery"
)

func provideDiscoveryClient(cfg *configs.Config) (*discovery.Client, error) {
	port, err := strconv.Atoi(cfg.ServerPort)
	if err != nil {
		return nil, fmt.Errorf("parsing server port for eureka registration: %w", err)
	}
	baseURL := fmt.Sprintf("http://%s:%s/eureka", cfg.EurekaHost, cfg.EurekaPort)
	return discovery.NewClient(baseURL, eurekaAppName, eurekaVipAddress, port)
}

// provideBackfillGate es el interruptor compartido entre el ciclo (que lo
// prende durante el backfill) y el middleware BackfillGate (que bloquea las
// rutas de signal-processing-service mientras esta prendido, ver
// router.go) -- un solo *atomic.Bool para todo el proceso.
func provideBackfillGate() *atomic.Bool {
	return &atomic.Bool{}
}

func provideOAuth(cfg *configs.Config) *tastytrade.OAuth {
	return tastytrade.NewOAuth(tastytrade.OAuthConfig{
		BaseURL:      cfg.TastyTradeBaseURL,
		ClientID:     cfg.TastyTradeClientID,
		ClientSecret: cfg.TastyTradeClientSecret,
		RefreshToken: cfg.TastyTradeRefreshToken,
	})
}

func provideCandlePool(cfg *configs.Config, qt *tastytrade.QuoteToken, oauth *tastytrade.OAuth) *tastytrade.CandlePool {
	urlFunc := qt.DxlinkURL
	if cfg.DxlinkURLOverride != "" {
		urlFunc = func() string { return cfg.DxlinkURLOverride }
	}
	connFactory := func(ctx context.Context) (*tastytrade.DxLinkConn, error) {
		conn := tastytrade.NewDxLinkConn(urlFunc, qt.Token)
		conn.OnSessionReset(func(sctx context.Context) error {
			return oauth.ResetSessions(sctx)
		})
		conn.OnSessionSaturated(oauth.MarkSessionsSaturated)
		conn.OnWaitForSessionCooldown(oauth.WaitForSessionCooldown)
		if err := conn.Connect(ctx); err != nil {
			conn.Close()
			return nil, err
		}
		return conn, nil
	}
	return tastytrade.NewCandlePoolWithChannels(connFactory, cfg.MaxCandlePoolConnections, cfg.MaxChannelsPerConn)
}

func provideGateway(oauth *tastytrade.OAuth, pool *tastytrade.CandlePool) out.MarketDataGateway {
	return tastytrade.NewGateway(oauth, pool)
}

func provideHealthHandler(pool *tastytrade.CandlePool) *handler.HealthHandler {
	return handler.NewHealthHandler(pool)
}

func provideTickerCikLookup(cfg *configs.Config) *secedgar.TickerCikLookup {
	return secedgar.NewTickerCikLookup(cfg.SecEdgarCacheDir)
}

func provideSharesOutstandingGateway(cfg *configs.Config, tickerCik *secedgar.TickerCikLookup) out.SharesOutstandingGateway {
	return secedgar.NewEdgarClient(tickerCik, cfg.SecEdgarCacheDir)
}

func provideInsiderOwnershipGateway(cfg *configs.Config) out.InsiderOwnershipGateway {
	return secedgar.NewInsiderOwnershipClient(cfg.SecEdgarCacheDir)
}

func provideBeneficialOwnersGateway(tickerCik *secedgar.TickerCikLookup) out.BeneficialOwnersGateway {
	return secedgar.NewBeneficialOwnersClient(tickerCik)
}

func provideShortInterestGateway() out.ShortInterestGateway {
	return finra.NewClient()
}

func provideProfileSharesGateway(pool *tastytrade.CandlePool) out.ProfileSharesGateway {
	return pool
}

// provideDayVolumeGateway expone el mismo pool bajo el puerto de volumen
// real del dia -- un solo objeto, dos puertos (mismo patron que
// provideOpenInterestGateway).
func provideDayVolumeGateway(pool *tastytrade.CandlePool) out.DayVolumeGateway {
	return pool
}

// provideOpenInterestGateway expone el mismo adaptador Gateway bajo el
// puerto de open interest -- un solo objeto, dos puertos (mismo patron que
// provideProfileSharesGateway con el pool).
func provideOpenInterestGateway(g out.MarketDataGateway) out.OpenInterestGateway {
	return g.(out.OpenInterestGateway)
}
