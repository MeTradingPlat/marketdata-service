package injector

import (
	"fmt"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/incoming/handler"
	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/external/tastytrade"
	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/repository/timescale"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/ingestion"
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
	checkErr(container.Provide(provideDxLinkConn))
	checkErr(container.Provide(tastytrade.NewCandlePool))
	checkErr(container.Provide(provideGateway))

	checkErr(container.Provide(ingestion.NewIngestCandlesService))
	checkErr(container.Provide(ingestion.NewGetCandlesService))

	checkErr(container.Provide(handler.NewCandlesHandler))
	checkErr(container.Provide(provideHealthHandler))

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

func provideDxLinkConn(cfg *configs.Config, qt *tastytrade.QuoteToken) *tastytrade.DxLinkConn {
	urlFunc := qt.DxlinkURL
	if cfg.DxlinkURLOverride != "" {
		urlFunc = func() string { return cfg.DxlinkURLOverride }
	}
	return tastytrade.NewDxLinkConn(urlFunc, qt.Token)
}

func provideGateway(oauth *tastytrade.OAuth, pool *tastytrade.CandlePool) out.MarketDataGateway {
	return tastytrade.NewGateway(oauth, pool)
}

func provideHealthHandler(conn *tastytrade.DxLinkConn) *handler.HealthHandler {
	return handler.NewHealthHandler(conn)
}
