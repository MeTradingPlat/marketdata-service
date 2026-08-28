package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/external/tastytrade"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/fundamentals"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/intraday"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs/injector"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs/router"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/discovery"
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
		fundamentalsRepo out.FundamentalsRepository,
		ingest in.IngestCandlesService,
		edgar out.SharesOutstandingGateway,
		insiders out.InsiderOwnershipGateway,
		beneficialOwners out.BeneficialOwnersGateway,
		finra out.ShortInterestGateway,
		profileShares out.ProfileSharesGateway,
		discoveryClient *discovery.Client,
		e *echo.Echo,
		r *router.Router,
		backfilling *atomic.Bool,
		snapshotTracker *intraday.SnapshotTracker,
		fundamentalsCache *fundamentals.FundamentalsCache,
		recentCache *livecandles.RecentCache,
	) {
		// signal.NotifyContext cancela ctx al recibir SIGTERM/SIGINT --
		// eso apaga limpio el pool de DB, las goroutines en vivo y el catch-up
		// diario, en vez de que Docker mate el proceso a la fuerza y deje
		// conexiones zombie en Postgres (confirmado en vivo: un docker stop
		// dejaba una sesion colgada que bloqueaba TRUNCATE/ALTER despues).
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
		defer stop()

		// fatalStartup cierra las conexiones DxLink que se alcanzaron a abrir
		// ANTES de morir -- sin esto, un fallo de arranque (ej. WarmUp que
		// choca con el limite de sesiones) salia con log.Fatal() sin pasar
		// por el cierre ordenado, dejando huerfanas exactamente las sesiones
		// parciales que la siguiente vuelta del restart iba a necesitar
		// libres. Confirmado en vivo el 2026-08-22: un WarmUp fallido por
		// limite de sesiones crasheaba sin liberar nada, y el reinicio
		// automatico de Docker volvia a chocar con el mismo limite.
		fatalStartup := func(err error, msg string) {
			gateway.ResetLiveConnections()
			log.Fatal().Err(err).Msg(msg)
		}

		if _, err := oauth.RefreshAccessToken(ctx); err != nil {
			fatalStartup(err, "failed to obtain initial TastyTrade access token")
		}
		if err := quoteToken.Refresh(ctx); err != nil {
			fatalStartup(err, "failed to obtain initial DxLink quote token")
		}
		tastytrade.StartProactiveRefresh(ctx, oauth, quoteToken)

		if err := pool.WarmUp(ctx); err != nil {
			fatalStartup(err, "failed to warm up candle pool")
		}

		// Red de seguridad: si ActiveSymbols fallara en la primera pasada del
		// ciclo, estos simbolos ya quedan rastreados igual.
		testSymbols := make([]domain.Symbol, len(cfg.TestSymbols))
		for i, s := range cfg.TestSymbols {
			testSymbols[i] = domain.Symbol{Symbol: s, Market: cfg.TestMarket}
		}
		if err := symbols.Upsert(ctx, testSymbols); err != nil {
			fatalStartup(err, "failed to track test symbols")
		}

		// Ciclo del universo completo -- D1 fase 1, H1 fase 2, M1 fase 3 --
		// corre en background (no bloquea el arranque del servidor HTTP) y se
		// repite en cada ventana de mantenimiento. Ver universe_cycle.go.
		StartUniverseCycle(ctx, cfg, gateway, symbols, candleRepo, fundamentalsRepo, ingest, edgar, insiders, finra, profileShares, backfilling, snapshotTracker, fundamentalsCache)
		StartLiveReconcileLoop(ctx, ingest, gateway, symbols)
		StartSaveRetryLoop(ctx, ingest)
		StartRecentCacheEvictLoop(ctx, recentCache)
		StartTradingStatusLoop(ctx, gateway, symbols, fundamentalsRepo, fundamentalsCache)
		StartBeneficialOwnersLoop(ctx, beneficialOwners, fundamentalsRepo)
		StartSessionResetLoop(ctx, cfg, oauth, gateway)

		r.Init()
		address := fmt.Sprintf(":%s", cfg.ServerPort)
		go func() {
			if err := e.Start(address); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatal().Err(err).Msg("server stopped unexpectedly")
			}
		}()
		go discovery.Run(ctx, discoveryClient)

		<-ctx.Done()
		log.Info().Msg("shutdown signal received, closing down")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := e.Shutdown(shutdownCtx); err != nil {
			log.Error().Err(err).Msg("error shutting down http server")
		}
		// Cierra las conexiones DxLink con un FIN explicito antes de morir --
		// si el proceso muere con los sockets abiertos, el servidor de
		// TastyTrade tarda en reclamar las sesiones y un swap de contenedor
		// deja sesiones huerfanas que saturan el limite ("number of user
		// sessions has exceeded the configured limit", confirmado en vivo el
		// 2026-08-18 con 5 swaps en una hora).
		gateway.ResetLiveConnections()
		dbPool.Close()
		log.Info().Msg("shutdown complete")
	})
	if err != nil {
		panic(err)
	}
}
