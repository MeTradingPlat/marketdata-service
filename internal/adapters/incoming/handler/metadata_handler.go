package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/labstack/echo/v4"
)

type MetadataHandler struct {
	symbols       in.GetSymbolsService
	markets       in.GetMarketsService
	timeframes    in.GetTimeframesService
	searchSymbols in.SearchSymbolsService
	symbolDetails in.GetSymbolDetailsService
}

func NewMetadataHandler(symbols in.GetSymbolsService, markets in.GetMarketsService, timeframes in.GetTimeframesService, searchSymbols in.SearchSymbolsService, symbolDetails in.GetSymbolDetailsService) *MetadataHandler {
	return &MetadataHandler{symbols: symbols, markets: markets, timeframes: timeframes, searchSymbols: searchSymbols, symbolDetails: symbolDetails}
}

const defaultSearchPageSize = 50

func (h *MetadataHandler) SearchSymbols(c echo.Context) error {
	var markets []string
	if q := c.QueryParam("markets"); q != "" {
		markets = strings.Split(q, ",")
	}

	page, err := strconv.Atoi(c.QueryParam("page"))
	if err != nil || page < 0 {
		page = 0
	}
	size, err := strconv.Atoi(c.QueryParam("size"))
	if err != nil || size <= 0 {
		size = defaultSearchPageSize
	}

	result, err := h.searchSymbols.Search(c.Request().Context(), c.QueryParam("q"), markets, page, size)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, result)
}

func (h *MetadataHandler) GetSymbolDetails(c echo.Context) error {
	symbol := c.Param("symbol")
	details, err := h.symbolDetails.GetSymbolDetails(c.Request().Context(), symbol)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	return c.JSON(http.StatusOK, details)
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
