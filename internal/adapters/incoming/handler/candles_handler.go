package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs"
	"github.com/labstack/echo/v4"
)

type CandlesHandler struct {
	service in.GetCandlesService
	current in.GetCurrentCandleService

	// batchSema: ver el comentario de MAX_CONCURRENT_BATCH_RESPONSES en
	// config.go -- acota cuantas respuestas de /historical/batch (~9MB sin
	// comprimir cada una) se arman en memoria a la vez.
	batchSema chan struct{}
}

func NewCandlesHandler(service in.GetCandlesService, current in.GetCurrentCandleService, cfg *configs.Config) *CandlesHandler {
	max := cfg.MaxConcurrentBatchResponses
	if max <= 0 {
		max = 4
	}
	return &CandlesHandler{service: service, current: current, batchSema: make(chan struct{}, max)}
}

// GetCurrentCandle sirve la vela EN FORMACION del simbolo+timeframe (query
// param timeframe, default M1) -- para el grafico en vivo; los consumidores
// de velas cerradas no la usan. 404 cuando el periodo todavia no tiene
// ningun tick real (no se fabrica una vela plana para cubrir el hueco).
func (h *CandlesHandler) GetCurrentCandle(c echo.Context) error {
	symbol := c.Param("symbol")
	timeframe := domain.Timeframe(c.QueryParam("timeframe"))
	if timeframe == "" {
		timeframe = domain.M1
	}
	if !timeframe.Valid() {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid timeframe")
	}
	bar, err := h.current.GetCurrentCandle(c.Request().Context(), symbol, timeframe)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "no se pudo armar la vela en formacion")
	}
	if bar == nil {
		return echo.NewHTTPError(http.StatusNotFound, "sin datos para la vela en formacion")
	}
	return c.JSON(http.StatusOK, bar)
}

func (h *CandlesHandler) GetCandles(c echo.Context) error {
	symbol := c.Param("symbol")
	timeframe := domain.Timeframe(c.QueryParam("timeframe"))
	if !timeframe.Valid() {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid or missing timeframe")
	}

	bars := 100
	if raw := c.QueryParam("bars"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid bars")
		}
		bars = parsed
	}

	var before *time.Time
	if raw := c.QueryParam("endDate"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid endDate")
		}
		before = &parsed
	}

	candles, err := h.service.GetCandles(c.Request().Context(), symbol, timeframe, bars, before)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, candles)
}
