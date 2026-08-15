package handler

import (
	"net/http"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/labstack/echo/v4"
)

type IntradayHandler struct {
	service in.GetIntradaySnapshotService
}

func NewIntradayHandler(service in.GetIntradaySnapshotService) *IntradayHandler {
	return &IntradayHandler{service: service}
}

func (h *IntradayHandler) GetSnapshot(c echo.Context) error {
	symbol := c.Param("symbol")
	snapshot, err := h.service.GetSnapshot(c.Request().Context(), symbol)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, snapshot)
}
