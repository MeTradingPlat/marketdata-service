package router

import (
	"github.com/MeTradingPlat/marketdata-service/internal/adapters/incoming/handler"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/middleware"
	"github.com/labstack/echo/v4"
)

type Router struct {
	echo         *echo.Echo
	candles      *handler.CandlesHandler
	health       *handler.HealthHandler
	intraday     *handler.IntradayHandler
	fundamentals *handler.FundamentalsHandler
	metadata     *handler.MetadataHandler
	candleWS     *handler.CandleWSHandler
}

func NewRouter(e *echo.Echo, candles *handler.CandlesHandler, health *handler.HealthHandler, intraday *handler.IntradayHandler, fundamentals *handler.FundamentalsHandler, metadata *handler.MetadataHandler, candleWS *handler.CandleWSHandler) *Router {
	return &Router{echo: e, candles: candles, health: health, intraday: intraday, fundamentals: fundamentals, metadata: metadata, candleWS: candleWS}
}

func (r *Router) Init() {
	r.echo.Use(middleware.RequestLogging)
	r.echo.Use(middleware.GatewayHeaderCheck)

	r.echo.GET("/marketdata/health", r.health.Health)
	r.echo.GET("/marketdata/historical/:symbol", r.candles.GetCandles)
	r.echo.GET("/marketdata/intraday/:symbol", r.intraday.GetSnapshot)
	r.echo.GET("/marketdata/fundamentals/:symbol", r.fundamentals.GetFundamentals)
	r.echo.GET("/marketdata/symbols", r.metadata.GetSymbols)
	r.echo.GET("/marketdata/markets", r.metadata.GetMarkets)
	r.echo.GET("/marketdata/timeframes", r.metadata.GetTimeframes)
	r.echo.GET("/ws/candles", r.candleWS.Handle)
}
