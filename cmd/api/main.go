package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/external/tastytrade"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs/injector"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs/router"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const shutdownTimeout = 10 * time.Second

func main() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	container := injector.BuildContainer()

	err := container.Invoke(func(
		cfg *configs.Config,
		oauth *tastytrade.OAuth,
		quoteToken *tastytrade.QuoteToken,
		pool *tastytrade.CandlePool,
		dbPool *pgxpool.Pool,
		gateway out.MarketDataGateway,
		candleRepo out.CandleRepository,
		symbols out.SymbolRepository,
		ingest in.IngestCandlesService,
		e *echo.Echo,
		r *router.Router,
	) {
		// signal.NotifyContext cancela ctx al recibir SIGTERM/SIGINT --
		// eso apaga limpio el pool de DB, las goroutines en vivo y el catch-up
		// diario, en vez de que Docker mate el proceso a la fuerza y deje
		// conexiones zombie en Postgres (confirmado en vivo: un docker stop
		// dejaba una sesion colgada que bloqueaba TRUNCATE/ALTER despues).
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
		defer stop()

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

		// Red de seguridad: si ActiveSymbols fallara en la primera pasada del
		// ciclo, estos simbolos ya quedan rastreados igual.
		testSymbols := make([]domain.Symbol, len(cfg.TestSymbols))
		for i, s := range cfg.TestSymbols {
			testSymbols[i] = domain.Symbol{Symbol: s, Market: cfg.TestMarket}
		}
		if err := symbols.Upsert(ctx, testSymbols); err != nil {
			log.Fatal().Err(err).Msg("failed to track test symbols")
		}

		// Ciclo del universo completo -- D1 fase 1, H1 fase 2, M1 fase 3 --
		// corre en background (no bloquea el arranque del servidor HTTP) y se
		// repite en cada ventana de mantenimiento. Ver universe_cycle.go.
		StartUniverseCycle(ctx, pool, gateway, symbols, candleRepo, ingest)

		r.Init()
		address := fmt.Sprintf(":%s", cfg.ServerPort)
		go func() {
			if err := e.Start(address); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatal().Err(err).Msg("server stopped unexpectedly")
			}
		}()

		<-ctx.Done()
		log.Info().Msg("shutdown signal received, closing down")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := e.Shutdown(shutdownCtx); err != nil {
			log.Error().Err(err).Msg("error shutting down http server")
		}
		dbPool.Close()
		log.Info().Msg("shutdown complete")
	})
	if err != nil {
		panic(err)
	}
}
