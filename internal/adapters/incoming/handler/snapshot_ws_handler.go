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

// SnapshotWSHandler expone /ws/snapshot -- la sesion intradia (OHLC,
// volumenes, precio actual) del simbolo suscripto, publicada cada vez que
// cierra una M1 nueva (ver ingestion.publishSnapshot). Mismo canal en el que
// ya viaja la vela cerrada por /ws/candles, pero con el dato ya resumido en
// vez de que el frontend tenga que acumularlo el mismo de las velas.
type SnapshotWSHandler struct {
	broadcaster *livecandles.Broadcaster[domain.IntradaySnapshot]
	upgrader    websocket.Upgrader
}

func NewSnapshotWSHandler(broadcaster *livecandles.Broadcaster[domain.IntradaySnapshot]) *SnapshotWSHandler {
	return &SnapshotWSHandler{
		broadcaster: broadcaster,
		// El chequeo de origen ya lo hace el Gateway (CORS centralizado, ver
		// CLAUDE.md) -- este servicio nunca recibe conexiones directas del navegador.
		upgrader: websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }},
	}
}

func (h *SnapshotWSHandler) Handle(c echo.Context) error {
	conn, err := h.upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		log.Error().Err(err).Msg("failed to upgrade snapshot ws connection")
		return err
	}
	session := newRelayWSSession(conn, h.broadcaster, snapshotMessage)
	session.run(c.Request().Context())
	return nil
}

func snapshotMessage(symbol string, snap domain.IntradaySnapshot) any {
	return dto.SnapshotMessage{Type: "snapshot", Symbol: symbol, Snapshot: snap}
}
