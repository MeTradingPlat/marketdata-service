package handler

import (
	"net/http"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

type CandleWSHandler struct {
	getCandles in.GetCandlesService
	current    in.GetCurrentCandleService
	// hub: UNA sola instancia para todo el handler (todas las conexiones WS
	// de /ws/candles la comparten) -- es lo que permite que dos sesiones
	// pidiendo el mismo (symbol, timeframe) reusen la misma agregacion en
	// vez de cada una calcular la suya (ver candle_aggregate_hub.go).
	hub      *candleAggregateHub
	upgrader websocket.Upgrader
}

func NewCandleWSHandler(getCandles in.GetCandlesService, current in.GetCurrentCandleService, broadcaster *livecandles.Broadcaster[domain.Candle]) *CandleWSHandler {
	return &CandleWSHandler{
		getCandles: getCandles,
		current:    current,
		hub:        newCandleAggregateHub(broadcaster, current),
		// El chequeo de origen ya lo hace el Gateway (CORS centralizado, ver
		// CLAUDE.md) -- este servicio nunca recibe conexiones directas del navegador.
		upgrader: websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
	}
}

func (h *CandleWSHandler) Handle(c echo.Context) error {
	conn, err := h.upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		log.Error().Err(err).Msg("failed to upgrade candle ws connection")
		return err
	}
	session := newWSSession(conn, h.getCandles, h.current, h.hub)
	session.client = c.QueryParam("client")
	session.run(c.Request().Context())
	return nil
}
