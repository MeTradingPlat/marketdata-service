package handler

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/labstack/echo/v4"
)

// depthProber es el metodo de diagnostico que solo el Gateway real de
// tastytrade implementa (ver ProbeMaxDepthWithWait) -- no forma parte de
// out.MarketDataGateway, asi que se resuelve con una asercion de tipo en
// vez de un puerto nuevo (evitaria tocar cada fake de ese puerto por una
// funcion que ningun flujo real usa, solo este diagnostico puntual).
type depthProber interface {
	ProbeMaxDepthWithWait(ctx context.Context, symbol string, timeframe domain.Timeframe, wait time.Duration) ([]domain.Candle, error)
}

// DebugProbeHandler mide cuanto tarda de verdad la rafaga historica
// completa de un simbolo, sin el limite de historyDeepWait -- confirmado en
// vivo el 2026-08-31: FXI/PFE/IBIT quedaron con M1 mucho mas corto que el
// resto del universo, cada uno cortado en una fecha distinta (la firma de
// un timeout cortando a mitad de una rafaga todavia activa). Corre contra
// la sesion YA autenticada del proceso en vivo -- una sesion OAuth nueva
// aparte arriesgaria el refresh_token (rota en cada uso) del proceso real.
type DebugProbeHandler struct {
	gateway out.MarketDataGateway
}

func NewDebugProbeHandler(gateway out.MarketDataGateway) *DebugProbeHandler {
	return &DebugProbeHandler{gateway: gateway}
}

func (h *DebugProbeHandler) ProbeDepth(c echo.Context) error {
	prober, ok := h.gateway.(depthProber)
	if !ok {
		return echo.NewHTTPError(http.StatusNotImplemented, "gateway does not support depth probing")
	}

	symbol := c.Param("symbol")
	timeframe := domain.Timeframe(c.QueryParam("timeframe"))
	if !timeframe.Valid() {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid or missing timeframe")
	}
	wait := 10 * time.Minute
	if v := c.QueryParam("waitSeconds"); v != "" {
		seconds, err := strconv.Atoi(v)
		if err != nil || seconds <= 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid waitSeconds")
		}
		wait = time.Duration(seconds) * time.Second
	}

	start := time.Now()
	candles, err := prober.ProbeMaxDepthWithWait(c.Request().Context(), symbol, timeframe, wait)
	elapsed := time.Since(start)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	result := map[string]interface{}{
		"symbol":       symbol,
		"timeframe":    timeframe,
		"waitBudget":   wait.String(),
		"elapsed":      elapsed.String(),
		"candleCount":  len(candles),
		"hitWaitLimit": elapsed >= wait,
	}
	if len(candles) > 0 {
		result["oldest"] = candles[0].Timestamp
		result["newest"] = candles[len(candles)-1].Timestamp
	}
	return c.JSON(http.StatusOK, result)
}
