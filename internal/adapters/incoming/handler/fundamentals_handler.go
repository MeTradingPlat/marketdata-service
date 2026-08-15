package handler

import (
	"net/http"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/labstack/echo/v4"
)

type FundamentalsHandler struct {
	service in.GetFundamentalsService
}

func NewFundamentalsHandler(service in.GetFundamentalsService) *FundamentalsHandler {
	return &FundamentalsHandler{service: service}
}

func (h *FundamentalsHandler) GetFundamentals(c echo.Context) error {
	symbol := c.Param("symbol")
	fundamentals, err := h.service.GetFundamentals(c.Request().Context(), symbol)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, fundamentals)
}
