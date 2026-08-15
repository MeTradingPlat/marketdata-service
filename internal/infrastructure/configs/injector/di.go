package injector

import (
	"context"
	"fmt"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/incoming/handler"
	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/external/tastytrade"
	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/repository/timescale"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/ingestion"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/intraday"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs/router"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs/server"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs/storage"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/dig"
)

func BuildContainer() *dig.Container {
	container := dig.New()

	checkErr(container.Provide(configs.Load))
	checkErr(container.Provide(provideTimescalePool))
	checkErr(container.Provide(provideCandleRepository))
	checkErr(container.Provide(provideSymbolRepository))

	checkErr(container.Provide(provideOAuth))
	checkErr(container.Provide(tastytrade.NewQuoteToken))
	checkErr(container.Provide(provideCandlePool))
	checkErr(container.Provide(provideGateway))

	checkErr(container.Provide(ingestion.NewIngestCandlesService))
	checkErr(container.Provide(ingestion.NewGetCandlesService))
	checkErr(container.Provide(intraday.NewGetIntradaySnapshotService))

	checkErr(container.Provide(handler.NewCandlesHandler))
	checkErr(container.Provide(provideHealthHandler))
	checkErr(container.Provide(handler.NewIntradayHandler))

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

func provideCandleRepository(pool *pgxpool.Pool) out.CandleRepository {
	return timescale.NewCandleRepository(pool)
}

func provideSymbolRepository(pool *pgxpool.Pool) out.SymbolRepository {
	return timescale.NewSymbolRepository(pool)
}

func provideOAuth(cfg *configs.Config) *tastytrade.OAuth {
	return tastytrade.NewOAuth(tastytrade.OAuthConfig{
		BaseURL:      cfg.TastyTradeBaseURL,
		ClientID:     cfg.TastyTradeClientID,
		ClientSecret: cfg.TastyTradeClientSecret,
		RefreshToken: cfg.TastyTradeRefreshToken,
	})
}

func provideCandlePool(cfg *configs.Config, qt *tastytrade.QuoteToken) *tastytrade.CandlePool {
	urlFunc := qt.DxlinkURL
	if cfg.DxlinkURLOverride != "" {
		urlFunc = func() string { return cfg.DxlinkURLOverride }
	}
	connFactory := func(ctx context.Context) (*tastytrade.DxLinkConn, error) {
		conn := tastytrade.NewDxLinkConn(urlFunc, qt.Token)
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
