package handler

import (
	"net/http"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

// FundamentalsWSHandler expone /ws/fundamentals -- los fundamentales del
// simbolo suscripto, publicados solo cuando FundamentalsCache.ReloadAll
// corre de verdad (barrido nocturno o el loop de trading status cada 15
// min, ver cmd/api/trading_status_loop.go). Canal separado y de baja
// frecuencia a proposito, distinto de /ws/snapshot: estos campos no
// cambian a la velocidad del precio.
type FundamentalsWSHandler struct {
	broadcaster *livecandles.Broadcaster[domain.Fundamentals]
	upgrader    websocket.Upgrader
}

func NewFundamentalsWSHandler(broadcaster *livecandles.Broadcaster[domain.Fundamentals]) *FundamentalsWSHandler {
	return &FundamentalsWSHandler{
		broadcaster: broadcaster,
		upgrader:    websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
	}
}

func (h *FundamentalsWSHandler) Handle(c echo.Context) error {
	conn, err := h.upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		log.Error().Err(err).Msg("failed to upgrade fundamentals ws connection")
		return err
	}
	session := newRelayWSSession(conn, h.broadcaster, fundamentalsMessage)
	session.run(c.Request().Context())
	return nil
}

func fundamentalsMessage(symbol string, f domain.Fundamentals) any {
	return dto.FundamentalsMessage{Type: "fundamentals", Symbol: symbol, Fundamentals: f}
}
