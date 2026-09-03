package router

import (
	"compress/gzip"
	"sync/atomic"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/incoming/handler"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/middleware"
	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"
)

type Router struct {
	echo           *echo.Echo
	candles        *handler.CandlesHandler
	health         *handler.HealthHandler
	intraday       *handler.IntradayHandler
	fundamentals   *handler.FundamentalsHandler
	metadata       *handler.MetadataHandler
	candleWS       *handler.CandleWSHandler
	snapshotWS     *handler.SnapshotWSHandler
	fundamentalsWS *handler.FundamentalsWSHandler
	prices         *handler.CurrentPricesHandler
	debugProbe     *handler.DebugProbeHandler
	backfilling    *atomic.Bool
}

func NewRouter(e *echo.Echo, candles *handler.CandlesHandler, health *handler.HealthHandler, intraday *handler.IntradayHandler, fundamentals *handler.FundamentalsHandler, metadata *handler.MetadataHandler, candleWS *handler.CandleWSHandler, snapshotWS *handler.SnapshotWSHandler, fundamentalsWS *handler.FundamentalsWSHandler, prices *handler.CurrentPricesHandler, debugProbe *handler.DebugProbeHandler, backfilling *atomic.Bool) *Router {
	return &Router{echo: e, candles: candles, health: health, intraday: intraday, fundamentals: fundamentals, metadata: metadata, candleWS: candleWS, snapshotWS: snapshotWS, fundamentalsWS: fundamentalsWS, prices: prices, debugProbe: debugProbe, backfilling: backfilling}
}

// Init durante el fill/refill deja pasar siempre las lecturas livianas
// (fundamentals, symbols, quotes) y limita en concurrencia solo las
// pesadas (historical/intraday/candles) -- ver middleware.BackfillGate.
// Diseño original del 2026-08-18, restaurado el 2026-09-03 en vez del
// bloqueo total por ruta que se probo primero ese mismo dia (bloqueaba
// quotes/fundamentals sin necesidad en un refill normal).
func (r *Router) Init() {
	r.echo.Use(middleware.RequestLogging)
	r.echo.Use(middleware.GatewayHeaderCheck)
	// Gzip: /marketdata/historical/batch devuelve hasta ~9MB de JSON crudo
	// para un lote de 700 simbolos x 150 barras (confirmado en vivo el
	// 2026-08-19) -- JSON con nombres de campo repetidos miles de veces
	// comprime muy bien. Echo solo comprime si el cliente manda
	// Accept-Encoding: gzip (signal-processing-service tambien lo pide
	// ahora, ver marketdata_client.py), asi que no rompe nada para
	// clientes que no lo pidan.
	//
	// Level: BestSpeed (1), no el default (6) -- confirmado en vivo el
	// 2026-08-20: con 4 escaneres concurrentes pidiendo el universo
	// completo, el CPU del VAIO (4 nucleos) se saturaba a 85%+ comprimiendo
	// esas respuestas, siendo el cuello de botella real (pg_stat_activity
	// mostraba casi cero actividad en BD al mismo tiempo). El trafico va
	// casi todo por la red interna de Docker entre marketdata-service y
	// signal-processing-service, donde el ancho de banda ahorrado por un
	// nivel mas alto vale mucho menos que el CPU que cuesta -- JSON
	// comprime casi igual de bien en nivel 1 que en 6.
	r.echo.Use(echoMiddleware.GzipWithConfig(echoMiddleware.GzipConfig{Level: gzip.BestSpeed}))
	r.echo.Use(middleware.BackfillGate(r.backfilling))

	r.echo.GET("/marketdata/health", r.health.Health)
	r.echo.GET("/marketdata/historical/:symbol", r.candles.GetCandles)
	r.echo.GET("/marketdata/candles/:symbol/current", r.candles.GetCurrentCandle)
	r.echo.POST("/marketdata/historical/batch", r.candles.GetCandlesBatch)
	r.echo.GET("/marketdata/intraday/:symbol", r.intraday.GetSnapshot)
	r.echo.GET("/marketdata/fundamentals/:symbol", r.fundamentals.GetFundamentals)
	r.echo.POST("/marketdata/fundamentals/realtime", r.fundamentals.GetFundamentalsRealtime)
	// La ruta sigue llamandose /quotes/rest por compatibilidad con
	// signal-processing-service ya desplegado -- ver current_prices_handler.go.
	r.echo.POST("/marketdata/quotes/rest", r.prices.GetCurrentPrices)
	r.echo.GET("/marketdata/symbols", r.metadata.GetSymbols)
	r.echo.GET("/marketdata/symbols/search", r.metadata.SearchSymbols)
	r.echo.GET("/marketdata/symbols/:symbol/details", r.metadata.GetSymbolDetails)
	r.echo.GET("/marketdata/markets", r.metadata.GetMarkets)
	r.echo.GET("/marketdata/timeframes", r.metadata.GetTimeframes)
	r.echo.GET("/ws/candles", r.candleWS.Handle)
	r.echo.GET("/ws/snapshot", r.snapshotWS.Handle)
	r.echo.GET("/ws/fundamentals", r.fundamentalsWS.Handle)
	// Diagnostico puntual (ver debug_probe_handler.go) -- no pensado para
	// llamarse seguido, un wait largo bloquea el request entero.
	r.echo.GET("/marketdata/debug/probe-depth/:symbol", r.debugProbe.ProbeDepth)
}
