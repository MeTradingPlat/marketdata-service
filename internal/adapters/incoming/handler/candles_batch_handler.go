package handler

import (
	"net/http"
	"sync"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
	"github.com/labstack/echo/v4"
)

const candlesBatchWorkers = 20

type candlesBatchRequest struct {
	Symbols   []string `json:"symbols"`
	Timeframe string   `json:"timeframe"`
	Bars      int      `json:"bars"`
}

// GetCandlesBatch es lo que llama signal-processing-service (fetch_candles)
// para pedir velas de muchos simbolos de una -- reusa GetCandlesService por
// simbolo (incluye agregacion de timeframes derivados) con concurrencia
// acotada, mismo criterio que liveRolloutWorkers para no saturar el pool
// de conexiones a Postgres con miles de queries de golpe.
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

	result := make(map[string][]domain.Candle, len(req.Symbols))
	var mu sync.Mutex
	jobs := make(chan string, len(req.Symbols))
	for _, s := range req.Symbols {
		jobs <- s
	}
	close(jobs)

	var wg sync.WaitGroup
	for i := 0; i < candlesBatchWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for symbol := range jobs {
				candles, err := h.service.GetCandles(c.Request().Context(), symbol, timeframe, bars, nil)
				if err != nil || len(candles) == 0 {
					continue
				}
				mu.Lock()
				result[symbol] = candles
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	return c.JSON(http.StatusOK, dto.CandlesBatchResponse{CandlesPorSimbolo: result})
}
