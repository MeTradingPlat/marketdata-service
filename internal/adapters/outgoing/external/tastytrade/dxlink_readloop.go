package tastytrade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// idleReadTimeout: intento de deadline sobre el propio Read() -- se deja
// puesto porque no hace daño, pero NO es la defensa real: confirmado en
// vivo con dos volcados de goroutines separados que SetReadDeadline de
// gorilla/websocket no fuerza el timeout de forma confiable en este
// entorno (el readLoop seguia bloqueado en Read() varios minutos despues
// de vencido, sin error). La deteccion real esta en healthCheckLoop
// (dxlink_reconnect.go), que cierra el socket a la fuerza desde otra
// goroutine si no llega nada por demasiado tiempo -- eso si garantiza que
// el Read() bloqueado reviente con error.
const idleReadTimeout = 60 * time.Second

func (c *DxLinkConn) readLoop(ctx context.Context) {
	for {
		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()
		if conn == nil {
			return
		}

		_ = conn.SetReadDeadline(time.Now().Add(idleReadTimeout))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			c.noteCloseReason(formatReadErrorReason(err))
			logReadError(ctx, err)
			c.notifyHandshakeFailure(err)
			c.cleanup()
			c.handleDisconnect(ctx)
			return
		}
		c.handleMessage(ctx, msg)
	}
}

func logReadError(ctx context.Context, err error) {
	if ctx.Err() != nil {
		return
	}
	var ce *websocket.CloseError
	if errors.As(err, &ce) {
		log.Warn().Int("close_code", ce.Code).Str("close_text", ce.Text).Msg("dxlink read error")
		return
	}
	log.Warn().Str("error", err.Error()).Msg("dxlink read error")
}

func (c *DxLinkConn) notifyHandshakeFailure(err error) {
	c.mu.Lock()
	done := c.handshakeDone
	c.handshakeDone = nil
	c.mu.Unlock()

	var ce *websocket.CloseError
	if errors.As(err, &ce) && ce.Code == websocket.CloseNormalClosure && done != nil {
		log.Warn().Int64("openSessions", openDxLinkSessions.Load()).Msg("dxlink session saturation detected during handshake (new connection rejected)")
		c.markSessionSaturated()
	}

	if done != nil {
		done <- fmt.Errorf("dxlink connection closed during handshake: %w", err)
	}
}

func (c *DxLinkConn) markAuthenticated() {
	c.mu.Lock()
	c.setAuthenticated(true)
	done := c.handshakeDone
	c.handshakeDone = nil
	c.mu.Unlock()
	if done != nil {
		done <- nil
	}
}

func (c *DxLinkConn) handleMessage(ctx context.Context, raw []byte) {
	c.touchLastMessage()

	var env inboundEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		log.Error().Err(err).Msg("dxlink: failed to decode message")
		return
	}

	if c.dispatchChannelMessage(env) || c.dispatchControlMessage(ctx, env) {
		return
	}
	c.handleUnknownMessage(env.Type)
}

func (c *DxLinkConn) handleError(ctx context.Context, env inboundEnvelope) {
	c.stats.recordServerError(env.Error, time.Now())
	switch env.Error {
	case "BAD_ACTION":
		log.Warn().Int64("connID", c.id).Int("channel", env.Channel).Str("message", env.Message).Msg("dxlink BAD_ACTION")
	case "UNAUTHORIZED":
		// El servidor puede invalidar la sesion sin cerrar el socket -- sin
		// forzar authenticated=false aca, checkConnectionHealth nunca lo
		// detecta y el servicio queda mudo hasta un restart manual.
		// "The number of user sessions has exceeded the configured limit":
		// sesiones huerfanas de conexiones previas saturaron el limite --
		// la reconexion debe limpiarlas via REST primero (OnSessionReset).
		c.handleUnauthorizedError(ctx, env)
	case "INVALID_MESSAGE":
		// Un mensaje rechazado (ej. "content length exceeded 65536 bytes",
		// limite documentado por dxFeed) deja la conexion en un estado que
		// nadie corrige -- sin esto, el canal seguia mudo hasta que
		// idleReadTimeout/staleConnectionThreshold lo notaban por las malas
		// (60-90s despues) y forzaban una reconexion que repetia la MISMA
		// resuscripcion rota, ciclando sin fin. Reconectar de una vez acorta
		// la ventana de silencio; el mensaje culpable ya quedo logueado por
		// logOutgoingMessageSize (ver dxlink_conn.go) para diagnosticar cual
		// suscripcion especifica lo causo.
		c.handleInvalidMessageError(ctx, env)
	default:
		log.Error().Int64("connID", c.id).Int("channel", env.Channel).Str("error", env.Error).Str("message", env.Message).Msg("dxlink error")
	}
}
