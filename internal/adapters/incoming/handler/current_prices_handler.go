package handler

import (
	"net/http"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/labstack/echo/v4"
)

type QuotesHandler struct {
	service in.GetCurrentPricesService
}

func NewQuotesHandler(service in.GetCurrentPricesService) *QuotesHandler {
	return &QuotesHandler{service: service}
}

// GetQuotes es lo que llama signal-processing-service (fetch_quotes) para
// el lote ya filtrado por fundamentales -- body es un array plano de
// simbolos, respuesta es {symbol: precio}, un simbolo sin precio
// disponible simplemente no aparece (Python lo trata como None via .get()).
func (h *QuotesHandler) GetQuotes(c echo.Context) error {
	var symbols []string
	if err := c.Bind(&symbols); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	prices := h.service.GetCurrentPrices(c.Request().Context(), symbols)
	return c.JSON(http.StatusOK, prices)
}
