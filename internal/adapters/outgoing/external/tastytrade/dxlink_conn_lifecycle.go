package tastytrade

import (
	"context"
	"fmt"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

func (c *DxLinkConn) Connect(ctx context.Context) error {
	if c.closing.Load() {
		return fmt.Errorf("dxlink connection closing")
	}
	dialCtx, dialCancel := c.withCloseCancel(ctx)
	defer dialCancel()
	if c.waitForSessionCooldown != nil {
		release, err := c.waitForSessionCooldown(dialCtx)
		if err != nil {
			return fmt.Errorf("waiting for session cooldown: %w", err)
		}
		defer release()
		if c.closing.Load() {
			return fmt.Errorf("dxlink connection closing")
		}
	}

	conn, _, err := dxLinkDialer.DialContext(dialCtx, c.urlFunc(), nil)
	if err != nil {
		return fmt.Errorf("dialing dxlink websocket: %w", err)
	}

	// El default de gorilla es 64KB: un mensaje mayor (el lote de perfiles
	// de 13k simbolos del refresh externo, o un FEED_DATA de un batch
	// profundo) mataba la conexion con "content length exceeded" y alimentaba
	// el storm de reconexion -- confirmado en vivo el 2026-08-18 (el refresh
	// externo de las 16:03 llego degradado por esto). 64MB da margen de
	// sobra para cualquier lote.
	conn.SetReadLimit(64 << 20)

	runCtx, handshakeDone, err := c.publishConn(ctx, conn)
	if err != nil {
		return err
	}

	go c.readLoop(runCtx)

	if err := c.send(setupMessage{
		Type: "SETUP", Channel: 0, Version: "0.1-go/1.0.0",
		KeepaliveTimeout: 60, AcceptKeepaliveTimeout: 60,
	}); err != nil {
		return fmt.Errorf("sending dxlink setup: %w", err)
	}

	if err := c.waitHandshake(ctx, handshakeDone); err != nil {
		return err
	}

	c.watchLoopsOnce.Do(func() {
		go c.keepaliveLoop(runCtx)
		go c.healthCheckLoop(runCtx)
	})
	c.reconnectMu.Lock()
	c.reconnectAttempts = 0
	c.reconnectMu.Unlock()
	return nil
}

// Close es un cierre INTENCIONAL -- a diferencia de cleanup() (usado por la
// deteccion de conexion zombie, que SI debe reconectar), esto marca la
// conexion para que scheduleReconnect no dispare una reconexion automatica.
// Se usa en las fronteras D1->H1->M1 del barrido para arrancar cada fase
// con cero sesiones abiertas ante TastyTrade.
func (c *DxLinkConn) Close() {
	c.noteCloseReason("intentional_close")
	c.closing.Store(true)
	if c.closedCh != nil {
		c.closeOnce.Do(func() { close(c.closedCh) })
	}
	c.sendCloseFrame()
	c.cleanup()
}

func (c *DxLinkConn) sendCloseFrame() {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return
	}
	frame := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
	_ = conn.WriteControl(websocket.CloseMessage, frame, time.Now().Add(closeFrameTimeout))
}

func (c *DxLinkConn) cleanup() {
	hadSocket, wasAuth, channelCount := c.cleanupLocked()
	if hadSocket {
		log.Info().
			Int64("connID", c.id).
			Str("reason", c.closeReasonText()).
			Bool("authenticated", wasAuth).
			Dur("last_message_age", c.lastMessageAge()).
			Dur("socket_lifetime", c.socketLifetime()).
			Int("channels", channelCount).
			Fields(c.stats.snapshot()).
			Msg("dxlink socket closed")
	}
}

func (c *DxLinkConn) cleanupLocked() (bool, bool, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	hadSocket := c.conn != nil
	wasAuth := c.authenticated
	channelCount := len(c.channels)
	c.setAuthenticated(false)
	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.conn = nil
	c.retireChannelsLocked()
	c.channels = make(map[int]*dxLinkChannel)
	if c.connDone != nil {
		close(c.connDone)
		c.connDone = nil
	}
	return hadSocket, wasAuth, channelCount
}

func (c *DxLinkConn) retireChannelsLocked() {
	for _, ch := range c.channels {
		ch.retired.Store(true)
	}
}
