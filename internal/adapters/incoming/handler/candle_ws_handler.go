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
	getCandles  in.GetCandlesService
	current     in.GetCurrentCandleService
	broadcaster *livecandles.Broadcaster[domain.Candle]
	upgrader    websocket.Upgrader
}

func NewCandleWSHandler(getCandles in.GetCandlesService, current in.GetCurrentCandleService, broadcaster *livecandles.Broadcaster[domain.Candle]) *CandleWSHandler {
	return &CandleWSHandler{
		getCandles:  getCandles,
		current:     current,
		broadcaster: broadcaster,
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
	session := newWSSession(conn, h.getCandles, h.current, h.broadcaster)
	session.run(c.Request().Context())
	return nil
}
