package handler

import (
	"net/http"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
	"github.com/labstack/echo/v4"
)

type candlesBatchRequest struct {
	Symbols   []string `json:"symbols"`
	Timeframe string   `json:"timeframe"`
	Bars      int      `json:"bars"`
}

// GetCandlesBatch es lo que llama signal-processing-service (fetch_candles)
// para pedir velas de muchos simbolos de una -- ver
// in.GetCandlesService.GetCandlesBatch sobre la estrategia (consulta en
// lote para M5/M15, concurrencia acotada por simbolo para el resto).
func (h *CandlesHandler) GetCandlesBatch(c echo.Context) error {
	var req candlesBatchRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	timeframe := domain.Timeframe(req.Timeframe)
	if !timeframe.Valid() {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid or missing timeframe")
	}
	bars := req.Bars
	if bars <= 0 {
		bars = 100
	}

	// Ver batchSema en candles_handler.go -- sin este tope, cada llamada
	// grande concurrente (varios scanners evaluando a la vez) apila su
	// propio buffer de ~9MB de JSON crudo en memoria al mismo tiempo, la
	// causa confirmada (dmesg) de dos de los OOM del contenedor.
	select {
	case h.batchSema <- struct{}{}:
		defer func() { <-h.batchSema }()
	case <-c.Request().Context().Done():
		return echo.NewHTTPError(http.StatusServiceUnavailable, "request cancelled while waiting for batch capacity")
	}

	result := h.service.GetCandlesBatch(c.Request().Context(), req.Symbols, timeframe, bars)
	return c.JSON(http.StatusOK, dto.CandlesBatchResponse{CandlesPorSimbolo: result})
}
