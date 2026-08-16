package handler

import (
	"net/http"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/labstack/echo/v4"
)

type FundamentalsHandler struct {
	service         in.GetFundamentalsService
	realtimeService in.GetFundamentalsRealtimeService
}

func NewFundamentalsHandler(service in.GetFundamentalsService, realtimeService in.GetFundamentalsRealtimeService) *FundamentalsHandler {
	return &FundamentalsHandler{service: service, realtimeService: realtimeService}
}

func (h *FundamentalsHandler) GetFundamentals(c echo.Context) error {
	symbol := c.Param("symbol")
	fundamentals, err := h.service.GetFundamentals(c.Request().Context(), symbol)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, fundamentals)
}

// GetFundamentalsRealtime es lo que llama signal-processing-service
// (fetch_fundamentals) -- body es un array plano de simbolos, respuesta es
// {symbol: FundamentalRealtime}.
func (h *FundamentalsHandler) GetFundamentalsRealtime(c echo.Context) error {
	var symbols []string
	if err := c.Bind(&symbols); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	result := h.realtimeService.GetFundamentalsRealtime(c.Request().Context(), symbols)
	return c.JSON(http.StatusOK, result)
}
