package router

import (
	"github.com/MeTradingPlat/marketdata-service/internal/adapters/incoming/handler"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/middleware"
	"github.com/labstack/echo/v4"
)

type Router struct {
	echo     *echo.Echo
	candles  *handler.CandlesHandler
	health   *handler.HealthHandler
	intraday *handler.IntradayHandler
}

func NewRouter(e *echo.Echo, candles *handler.CandlesHandler, health *handler.HealthHandler, intraday *handler.IntradayHandler) *Router {
	return &Router{echo: e, candles: candles, health: health, intraday: intraday}
}

func (r *Router) Init() {
	r.echo.Use(middleware.RequestLogging)
	r.echo.Use(middleware.GatewayHeaderCheck)

	r.echo.GET("/marketdata/health", r.health.Health)
	r.echo.GET("/marketdata/historical/:symbol", r.candles.GetCandles)
	r.echo.GET("/marketdata/intraday/:symbol", r.intraday.GetSnapshot)
}
