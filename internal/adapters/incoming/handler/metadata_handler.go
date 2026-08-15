package handler

import (
	"net/http"
	"strings"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/labstack/echo/v4"
)

type MetadataHandler struct {
	symbols    in.GetSymbolsService
	markets    in.GetMarketsService
	timeframes in.GetTimeframesService
}

func NewMetadataHandler(symbols in.GetSymbolsService, markets in.GetMarketsService, timeframes in.GetTimeframesService) *MetadataHandler {
	return &MetadataHandler{symbols: symbols, markets: markets, timeframes: timeframes}
}

func (h *MetadataHandler) GetSymbols(c echo.Context) error {
	var markets []string
	if q := c.QueryParam("markets"); q != "" {
		markets = strings.Split(q, ",")
	}
	symbols, err := h.symbols.GetSymbols(c.Request().Context(), markets)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, symbols)
}

func (h *MetadataHandler) GetMarkets(c echo.Context) error {
	markets, err := h.markets.GetMarkets(c.Request().Context())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, markets)
}

func (h *MetadataHandler) GetTimeframes(c echo.Context) error {
	return c.JSON(http.StatusOK, h.timeframes.GetTimeframes())
}
