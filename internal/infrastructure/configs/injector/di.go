package injector

import (
	"context"
	"fmt"
	"strconv"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/incoming/handler"
	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/external/finra"
	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/external/secedgar"
	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/external/tastytrade"
	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/repository/timescale"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/fundamentals"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/ingestion"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/intraday"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/metadata"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs/router"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs/server"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs/storage"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/discovery"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/dig"
)

const (
	eurekaAppName    = "MARKETDATA-SERVICE"
	eurekaVipAddress = "marketdata-service"
)

func BuildContainer() *dig.Container {
	container := dig.New()

	checkErr(container.Provide(configs.Load))
	checkErr(container.Provide(provideTimescalePool))
	checkErr(container.Provide(provideTimescaleWritePool))
	checkErr(container.Provide(provideTimescaleSnapshotPool))
	checkErr(container.Provide(provideCandleRepository))
	checkErr(container.Provide(provideSymbolRepository))
	checkErr(container.Provide(provideFundamentalsRepository))

	checkErr(container.Provide(provideDiscoveryClient))
	checkErr(container.Provide(livecandles.NewBroadcaster[domain.Candle]))
	checkErr(container.Provide(livecandles.NewBroadcaster[domain.IntradaySnapshot]))
	checkErr(container.Provide(livecandles.NewBroadcaster[domain.Fundamentals]))
	checkErr(container.Provide(livecandles.NewDefaultRecentCache))
	checkErr(container.Provide(intraday.NewSnapshotTracker))

	checkErr(container.Provide(provideTickerCikLookup))
	checkErr(container.Provide(provideSharesOutstandingGateway))
	checkErr(container.Provide(provideInsiderOwnershipGateway))
	checkErr(container.Provide(provideBeneficialOwnersGateway))
	checkErr(container.Provide(provideShortInterestGateway))
	checkErr(container.Provide(provideProfileSharesGateway))
	checkErr(container.Provide(provideOpenInterestGateway))

	checkErr(container.Provide(provideOAuth))
	checkErr(container.Provide(tastytrade.NewQuoteToken))
	checkErr(container.Provide(provideCandlePool))
	checkErr(container.Provide(provideGateway))

	checkErr(container.Provide(ingestion.NewIngestCandlesService))
	checkErr(container.Provide(ingestion.NewGetCandlesService))
	checkErr(container.Provide(livecandles.NewCurrentCandleService))
	checkErr(container.Provide(intraday.NewGetIntradaySnapshotService))
	checkErr(container.Provide(intraday.NewGetCurrentPricesService))
	checkErr(container.Provide(fundamentals.NewFundamentalsCache))
	checkErr(container.Provide(fundamentals.NewGetFundamentalsService))
	checkErr(container.Provide(fundamentals.NewGetFundamentalsRealtimeService))
	checkErr(container.Provide(metadata.NewSymbolsCache))
	checkErr(container.Provide(metadata.NewGetSymbolsService))
	checkErr(container.Provide(metadata.NewGetMarketsService))
	checkErr(container.Provide(metadata.NewGetTimeframesService))
	checkErr(container.Provide(metadata.NewSearchSymbolsService))
	checkErr(container.Provide(metadata.NewGetSymbolDetailsService))

	checkErr(container.Provide(handler.NewCandlesHandler))
	checkErr(container.Provide(provideHealthHandler))
	checkErr(container.Provide(handler.NewIntradayHandler))
	checkErr(container.Provide(handler.NewFundamentalsHandler))
	checkErr(container.Provide(handler.NewMetadataHandler))
	checkErr(container.Provide(handler.NewCandleWSHandler))
	checkErr(container.Provide(handler.NewSnapshotWSHandler))
	checkErr(container.Provide(handler.NewFundamentalsWSHandler))
	checkErr(container.Provide(handler.NewCurrentPricesHandler))
	checkErr(container.Provide(handler.NewDebugProbeHandler))

	checkErr(container.Provide(server.NewServer))
	checkErr(container.Provide(router.NewRouter))

	return container
}

func checkErr(err error) {
	if err != nil {
		panic(fmt.Sprintf("error injecting: %v", err))
	}
}

func provideTimescalePool(cfg *configs.Config) *pgxpool.Pool {
	return storage.ConnInstanceTimescale(cfg)
}

// writePool es un tipo nominal para que dig distinga el pool de escritura
// del de lectura -- ambos son *pgxpool.Pool por debajo, y sin un tipo
// distinto dig inyectaria el mismo pool en los dos parametros.
type writePool struct{ pool *pgxpool.Pool }

// provideTimescaleWritePool es el pool chico dedicado a escrituras de velas
// (ver storage.writePoolConns).
func provideTimescaleWritePool(cfg *configs.Config) writePool {
	return writePool{pool: storage.WritePoolInstanceTimescale(cfg)}
}

// snapshotPool es un tipo nominal, mismo motivo que writePool: distinguirlo
// del resto de *pgxpool.Pool para que dig no inyecte el pool equivocado.
type snapshotPool struct{ pool *pgxpool.Pool }

// provideTimescaleSnapshotPool es el pool chico dedicado a
// fundamentals/realtime en lote (ver storage.snapshotPoolConns).
func provideTimescaleSnapshotPool(cfg *configs.Config) snapshotPool {
	return snapshotPool{pool: storage.SnapshotPoolInstanceTimescale(cfg)}
}

func provideCandleRepository(pool *pgxpool.Pool, writePool writePool, snapshotPool snapshotPool) out.CandleRepository {
	return timescale.NewCandleRepository(pool, writePool.pool, snapshotPool.pool)
}

func provideSymbolRepository(pool *pgxpool.Pool) out.SymbolRepository {
	return timescale.NewSymbolRepository(pool)
}

func provideFundamentalsRepository(pool *pgxpool.Pool) out.FundamentalsRepository {
	return timescale.NewFundamentalsRepository(pool)
}

func provideDiscoveryClient(cfg *configs.Config) (*discovery.Client, error) {
	port, err := strconv.Atoi(cfg.ServerPort)
	if err != nil {
		return nil, fmt.Errorf("parsing server port for eureka registration: %w", err)
	}
	baseURL := fmt.Sprintf("http://%s:%s/eureka", cfg.EurekaHost, cfg.EurekaPort)
	return discovery.NewClient(baseURL, eurekaAppName, eurekaVipAddress, port)
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
			return nil, err
		}
		return conn, nil
	}
	return tastytrade.NewCandlePool(connFactory, cfg.MaxCandlePoolConnections)
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

// provideOpenInterestGateway expone el mismo adaptador Gateway bajo el
// puerto de open interest -- un solo objeto, dos puertos (mismo patron que
// provideProfileSharesGateway con el pool).
func provideOpenInterestGateway(g out.MarketDataGateway) out.OpenInterestGateway {
	return g.(out.OpenInterestGateway)
}
