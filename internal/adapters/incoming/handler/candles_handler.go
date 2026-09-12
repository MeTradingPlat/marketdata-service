package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

// lightBatchMaxSymbols: por debajo de este tamano, un request de
// /historical/batch (ej. pivots del frontend, 1 simbolo) usa lightBatchSema
// en vez de batchSema -- confirmado en vivo el 2026-09-12 que un pivots de
// UN simbolo tardaba 14-15s por request porque hacia fila detras de los
// escaneres (cientos/miles de simbolos) por el mismo cupo de 4. El riesgo de
// memoria que batchSema evita (~9MB sin comprimir por respuesta) no aplica a
// un puñado de simbolos, asi que no tiene sentido que compitan por el mismo
// semaforo.
const lightBatchMaxSymbols = 10
const lightBatchConcurrency = 16

type CandlesHandler struct {
	service in.GetCandlesService
	current in.GetCurrentCandleService

	// batchSema: ver el comentario de MAX_CONCURRENT_BATCH_RESPONSES en
	// config.go -- acota cuantas respuestas GRANDES de /historical/batch
	// (~9MB sin comprimir cada una) se arman en memoria a la vez.
	batchSema chan struct{}
	// lightBatchSema: ver lightBatchMaxSymbols -- cupo aparte, mas generoso,
	// para requests chicos que no deben esperar detras de un escaner.
	lightBatchSema chan struct{}
}

func NewCandlesHandler(service in.GetCandlesService, current in.GetCurrentCandleService, cfg *configs.Config) *CandlesHandler {
	max := cfg.MaxConcurrentBatchResponses
	if max <= 0 {
		max = 4
	}
	return &CandlesHandler{
		service:        service,
		current:        current,
		batchSema:      make(chan struct{}, max),
		lightBatchSema: make(chan struct{}, lightBatchConcurrency),
	}
}

// GetCurrentCandle sirve la vela EN FORMACION del simbolo+timeframe (query
// param timeframe, default M1) -- para el grafico en vivo; los consumidores
// de velas cerradas no la usan. 404 cuando el periodo todavia no tiene
// ningun tick real (no se fabrica una vela plana para cubrir el hueco).
func (h *CandlesHandler) GetCurrentCandle(c echo.Context) error {
	symbol := c.Param("symbol")
	if !domain.ValidSymbolFormat(symbol) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid symbol")
	}
	timeframe := domain.Timeframe(c.QueryParam("timeframe"))
	if timeframe == "" {
		timeframe = domain.M1
	}
	if !timeframe.Valid() {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid timeframe")
	}
	bar, err := h.current.GetCurrentCandle(c.Request().Context(), symbol, timeframe)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "no se pudo armar la vela en formacion")
	}
	if bar == nil {
		return echo.NewHTTPError(http.StatusNotFound, "sin datos para la vela en formacion")
	}
	return c.JSON(http.StatusOK, bar)
}

func (h *CandlesHandler) GetCandles(c echo.Context) error {
	symbol := c.Param("symbol")
	if !domain.ValidSymbolFormat(symbol) {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid symbol")
	}
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

	var before *time.Time
	if raw := c.QueryParam("endDate"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid endDate")
		}
		before = &parsed
	}

	candles, err := h.service.GetCandles(c.Request().Context(), symbol, timeframe, bars, before)
	if err != nil {
		log.Error().Err(err).Str("symbol", symbol).Str("timeframe", string(timeframe)).Msg("failed to get candles")
		return echo.NewHTTPError(http.StatusInternalServerError, "no se pudieron obtener las candles")
	}
	return c.JSON(http.StatusOK, candles)
}
