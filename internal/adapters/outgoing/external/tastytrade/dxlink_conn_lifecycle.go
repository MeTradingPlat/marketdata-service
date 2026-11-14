package tastytrade

import (
	"context"
	"fmt"
	"time"

	"github.com/gorilla/websocket"
)

func (c *DxLinkConn) Connect(ctx context.Context) error {
	if c.waitForSessionCooldown != nil {
		release, err := c.waitForSessionCooldown(ctx)
		if err != nil {
			return fmt.Errorf("waiting for session cooldown: %w", err)
		}
		defer release()
	}

	conn, _, err := dxLinkDialer.DialContext(ctx, c.urlFunc(), nil)
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

	handshakeDone := make(chan error, 1)
	c.mu.Lock()
	if c.ctx == nil || c.ctx.Err() != nil {
		c.ctx = ctx
	}
	runCtx := c.ctx
	c.conn = conn
	c.setAuthenticated(false)
	c.handshakeDone = handshakeDone
	c.channels = make(map[int]*dxLinkChannel)
	c.connDone = make(chan struct{})
	c.mu.Unlock()
	c.touchLastMessage()

	go c.readLoop(runCtx)

	if err := c.send(setupMessage{
		Type: "SETUP", Channel: 0, Version: "0.1-go/1.0.0",
		KeepaliveTimeout: 60, AcceptKeepaliveTimeout: 60,
	}); err != nil {
		return fmt.Errorf("sending dxlink setup: %w", err)
	}

	select {
	case err := <-handshakeDone:
		if err != nil {
			return err
		}
	case <-time.After(handshakeTimeout):
		// Sin este cleanup, el socket recien abierto (AUTH nunca llego a
		// AUTHORIZED) y su readLoop quedan colgando sin ningun vigia: el
		// healthCheckLoop que detecta conexiones zombie solo arranca DESPUES
		// de un handshake exitoso (mas abajo), asi que nadie mas los cierra.
		c.cleanup()
		return fmt.Errorf("dxlink handshake timed out")
	case <-ctx.Done():
		c.cleanup()
		return ctx.Err()
	}

	go c.keepaliveLoop(runCtx)
	go c.healthCheckLoop(runCtx)
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
	c.mu.Lock()
	defer c.mu.Unlock()
	c.setAuthenticated(false)
	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.conn = nil
	c.channels = make(map[int]*dxLinkChannel)
	if c.connDone != nil {
		close(c.connDone)
		c.connDone = nil
	}
}
