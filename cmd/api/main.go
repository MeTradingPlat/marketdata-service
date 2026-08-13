package main

import (
	"context"
	"fmt"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/external/tastytrade"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs/injector"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs/router"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	container := injector.BuildContainer()

	err := container.Invoke(func(
		cfg *configs.Config,
		oauth *tastytrade.OAuth,
		quoteToken *tastytrade.QuoteToken,
		conn *tastytrade.DxLinkConn,
		symbols out.SymbolRepository,
		ingest in.IngestCandlesService,
		e *echo.Echo,
		r *router.Router,
	) {
		ctx := context.Background()

		if _, err := oauth.RefreshAccessToken(ctx); err != nil {
			log.Fatal().Err(err).Msg("failed to obtain initial TastyTrade access token")
		}
		if err := quoteToken.Refresh(ctx); err != nil {
			log.Fatal().Err(err).Msg("failed to obtain initial DxLink quote token")
		}
		tastytrade.StartProactiveRefresh(ctx, oauth, quoteToken)

		if err := conn.Connect(ctx); err != nil {
			log.Fatal().Err(err).Msg("failed to connect to DxLink")
		}

		if err := symbols.Upsert(ctx, []domain.Symbol{{Symbol: cfg.TestSymbol, Market: cfg.TestMarket}}); err != nil {
			log.Fatal().Err(err).Msg("failed to track test symbol")
		}
		if err := ingest.StreamLive(ctx, cfg.TestSymbol); err != nil {
			log.Error().Err(err).Str("symbol", cfg.TestSymbol).Msg("failed to start live candle stream")
		}
		for _, tf := range []domain.Timeframe{domain.D1, domain.H1, domain.M1} {
			if err := ingest.Backfill(ctx, cfg.TestSymbol, tf); err != nil {
				log.Error().Err(err).Str("symbol", cfg.TestSymbol).Str("timeframe", string(tf)).Msg("failed to backfill history")
			} else {
				log.Info().Str("symbol", cfg.TestSymbol).Str("timeframe", string(tf)).Msg("backfill finished")
			}
		}

		r.Init()
		address := fmt.Sprintf(":%s", cfg.ServerPort)
		log.Fatal().Err(e.Start(address)).Msg("server stopped")
	})
	if err != nil {
		panic(err)
	}
}
