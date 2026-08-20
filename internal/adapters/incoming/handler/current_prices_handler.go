package handler

import (
	"net/http"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/labstack/echo/v4"
)

// CurrentPricesHandler expone GetCurrentPricesService por HTTP -- la ruta
// sigue llamandose /marketdata/quotes/rest por compatibilidad con el
// contrato ya desplegado, pero la respuesta es solo {symbol: precio}, nunca
// un bid/ask real (eso no existe en este servicio todavia).
type CurrentPricesHandler struct {
	service in.GetCurrentPricesService
}

func NewCurrentPricesHandler(service in.GetCurrentPricesService) *CurrentPricesHandler {
	return &CurrentPricesHandler{service: service}
}

// GetCurrentPrices es lo que llama signal-processing-service
// (fetch_current_prices) para el lote ya filtrado por fundamentales -- body
// es un array plano de simbolos, respuesta es {symbol: precio}, un simbolo
// sin precio disponible simplemente no aparece (Python lo trata como None
// via .get()).
func (h *CurrentPricesHandler) GetCurrentPrices(c echo.Context) error {
	var symbols []string
	if err := c.Bind(&symbols); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	prices := h.service.GetCurrentPrices(c.Request().Context(), symbols)
	return c.JSON(http.StatusOK, prices)
}
