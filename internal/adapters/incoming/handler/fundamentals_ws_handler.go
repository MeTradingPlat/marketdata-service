package handler

import (
	"context"
	"net/http"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

// FundamentalsWSHandler expone /ws/fundamentals -- los fundamentales del
// simbolo suscripto, publicados cada vez que un refresco puntual de
// FundamentalsCache corre para ese simbolo (beta/market metrics/earnings/
// dividendos/externo/prevClose), ademas del barrido de ReloadAll. Canal
// separado y de baja frecuencia a proposito, distinto de /ws/snapshot: estos
// campos no cambian a la velocidad del precio.
type FundamentalsWSHandler struct {
	broadcaster *livecandles.Broadcaster[domain.Fundamentals]
	// realtimeService arma el mismo FundamentalRealtime que ya sirve
	// /marketdata/fundamentals/realtime -- el broadcaster solo trae
	// domain.Fundamentals crudo (sharesOutstanding/floatShares/shortInterest/
	// shortRatio/daysUntilEarnings son json:"-" ahi, nunca viajarian tal
	// cual). Un lookup de 1 simbolo contra GetFundamentalsRealtime sale de
	// las mismas cachés en memoria que ya usa el REST, sin I/O real.
	realtimeService in.GetFundamentalsRealtimeService
	upgrader        websocket.Upgrader
}

func NewFundamentalsWSHandler(broadcaster *livecandles.Broadcaster[domain.Fundamentals], realtimeService in.GetFundamentalsRealtimeService) *FundamentalsWSHandler {
	return &FundamentalsWSHandler{
		broadcaster:     broadcaster,
		realtimeService: realtimeService,
		upgrader:        websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
	}
}

func (h *FundamentalsWSHandler) Handle(c echo.Context) error {
	conn, err := h.upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		log.Error().Err(err).Msg("failed to upgrade fundamentals ws connection")
		return err
	}
	session := newRelayWSSession(conn, h.broadcaster, h.fundamentalsMessage)
	session.run(c.Request().Context())
	return nil
}

func (h *FundamentalsWSHandler) fundamentalsMessage(symbol string, _ domain.Fundamentals) any {
	realtime := h.realtimeService.GetFundamentalsRealtime(context.Background(), []string{symbol})[symbol]
	return dto.FundamentalsMessage{Type: "fundamentals", Symbol: symbol, Fundamentals: realtime}
}
