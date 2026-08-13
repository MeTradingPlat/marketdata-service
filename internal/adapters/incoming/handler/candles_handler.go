package handler

import (
	"net/http"
	"strconv"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/labstack/echo/v4"
)

type CandlesHandler struct {
	service in.GetCandlesService
}

func NewCandlesHandler(service in.GetCandlesService) *CandlesHandler {
	return &CandlesHandler{service: service}
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

	candles, err := h.service.GetCandles(c.Request().Context(), symbol, timeframe, bars)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, candles)
}
