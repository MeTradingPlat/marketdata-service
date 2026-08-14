package main

import (
	"context"
	"fmt"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/external/tastytrade"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/aggregation"
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
		pool *tastytrade.CandlePool,
		gateway out.MarketDataGateway,
		symbols out.SymbolRepository,
		candles out.CandleRepository,
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

		if err := pool.WarmUp(ctx); err != nil {
			log.Fatal().Err(err).Msg("failed to warm up candle pool")
		}

		aggregation.StartForwardAggregation(ctx, candles)

		if cfg.BackfillBatchSize > 0 {
			runBackfillBatch(ctx, gateway, symbols, ingest, cfg.BackfillBatchSize, cfg.BackfillWorkers)
		}

		if cfg.UnsubscribeCheckSymbol != "" {
			if err := symbols.Upsert(ctx, []domain.Symbol{{Symbol: cfg.UnsubscribeCheckSymbol, Market: cfg.TestMarket}}); err != nil {
				log.Fatal().Err(err).Msg("failed to track unsubscribe-check symbol")
			}
			runUnsubscribeCheck(ctx, ingest, cfg.UnsubscribeCheckSymbol, cfg.UnsubscribeCheckRounds)
		}

		testSymbols := make([]domain.Symbol, len(cfg.TestSymbols))
		for i, s := range cfg.TestSymbols {
			testSymbols[i] = domain.Symbol{Symbol: s, Market: cfg.TestMarket}
		}
		if err := symbols.Upsert(ctx, testSymbols); err != nil {
			log.Fatal().Err(err).Msg("failed to track test symbols")
		}
		// Backfill primero, en vivo despues -- ver comentario en
		// CandlePool.SubscribeLive sobre por que no pueden ser concurrentes
		// para el mismo simbolo+M1.
		for _, symbol := range cfg.TestSymbols {
			for _, tf := range []domain.Timeframe{domain.D1, domain.H1, domain.M1} {
				if err := ingest.Backfill(ctx, symbol, tf); err != nil {
					log.Error().Err(err).Str("symbol", symbol).Str("timeframe", string(tf)).Msg("failed to backfill history")
				} else {
					log.Info().Str("symbol", symbol).Str("timeframe", string(tf)).Msg("backfill finished")
				}
			}
			if err := ingest.StreamLive(ctx, symbol); err != nil {
				log.Error().Err(err).Str("symbol", symbol).Msg("failed to start live candle stream")
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
