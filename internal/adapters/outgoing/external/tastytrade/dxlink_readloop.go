package tastytrade

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

// idleReadTimeout: si no llega NADA del servidor (dato real o su propio
// KEEPALIVE) en este tiempo, ReadMessage devuelve un timeout y cae por el
// mismo camino de error que un cierre de socket real. Sin este deadline,
// Read() se queda bloqueado para siempre ante una conexion zombie (TCP
// nunca cerrado, servidor mudo) -- confirmado en vivo, el streaming en vivo
// se quedo callado 5+ minutos sin ningun error ni intento de reconexion.
// 60s, no los 120s del DefaultMaxSessionIdleTimeout de Java -- ya estamos
// recibiendo velas en formacion en vivo, asi que esperar tanto para
// detectar el corte genera huecos de mas. El propio SETUP ya declara
// KeepaliveTimeout/AcceptKeepaliveTimeout=60, asi que 60s es el limite que
// el protocolo mismo espera, no un numero arbitrario -- y es 2x nuestro
// propio intervalo de envio de KEEPALIVE (30s), siguiendo el patron
// estandar de gorilla/websocket de que el deadline de lectura sea
// notablemente mayor al periodo de ping, nunca igual.
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
			c.notifyHandshakeFailure(err)
			c.handleDisconnect(ctx)
			return
		}
		c.handleMessage(ctx, msg)
	}
}

func (c *DxLinkConn) notifyHandshakeFailure(err error) {
	c.mu.Lock()
	done := c.handshakeDone
	c.handshakeDone = nil
	c.mu.Unlock()
	if done != nil {
		done <- fmt.Errorf("dxlink connection closed during handshake: %w", err)
	}
}

func (c *DxLinkConn) markAuthenticated() {
	c.mu.Lock()
	c.authenticated = true
	done := c.handshakeDone
	c.handshakeDone = nil
	c.mu.Unlock()
	if done != nil {
		done <- nil
	}
}

func (c *DxLinkConn) handleMessage(ctx context.Context, raw []byte) {
	var env inboundEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		log.Error().Err(err).Msg("dxlink: failed to decode message")
		return
	}

	switch env.Type {
	case "SETUP":
		if err := c.send(authMessage{Type: "AUTH", Channel: 0, Token: c.tokenFunc()}); err != nil {
			log.Error().Err(err).Msg("dxlink: failed to send AUTH")
		}
	case "AUTH_STATE":
		if env.State == "AUTHORIZED" {
			c.markAuthenticated()
		}
	case "CHANNEL_OPENED":
		if env.Service == "FEED" {
			if ch := c.channel(env.Channel); ch != nil {
				if err := ch.handleOpened(); err != nil {
					log.Error().Err(err).Int("channel", env.Channel).Msg("dxlink: failed to send FEED_SETUP")
				}
			}
		}
	case "FEED_CONFIG":
		if ch := c.channel(env.Channel); ch != nil {
			ch.handleConfigured()
		}
	case "FEED_DATA":
		if ch := c.channel(env.Channel); ch != nil {
			ch.handleData(env.Data)
		}
	case "KEEPALIVE":
		if err := c.send(keepaliveMessage{Type: "KEEPALIVE", Channel: 0}); err != nil {
			log.Error().Err(err).Msg("dxlink: failed to reply KEEPALIVE")
		}
	case "ERROR":
		c.handleError(ctx, env)
	}
}

func (c *DxLinkConn) handleError(ctx context.Context, env inboundEnvelope) {
	switch env.Error {
	case "BAD_ACTION":
		log.Debug().Str("message", env.Message).Msg("dxlink BAD_ACTION")
	case "UNAUTHORIZED":
		// El servidor puede invalidar la sesion sin cerrar el socket -- sin
		// forzar authenticated=false aca, checkConnectionHealth nunca lo
		// detecta y el servicio queda mudo hasta un restart manual.
		log.Error().Str("message", env.Message).Msg("dxlink UNAUTHORIZED, forcing reconnect")
		c.mu.Lock()
		c.authenticated = false
		c.mu.Unlock()
		c.scheduleReconnect(ctx)
	default:
		log.Error().Str("error", env.Error).Str("message", env.Message).Msg("dxlink error")
	}
}
