package tastytrade

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
)

func (c *DxLinkConn) performReconnect(ctx context.Context) error {
	if c.Connected() {
		return nil
	}
	// Sesiones huerfanas saturando el limite: cerrarlas via REST (logout +
	// refresh del token) antes del handshake -- si el logout falla se
	// reintenta igual: el backoff a 5 min y las sesiones que expiran solas
	// siguen siendo la red de seguridad.
	if c.sessionSaturated.Swap(false) && c.onSessionReset != nil {
		if err := c.onSessionReset(ctx); err != nil {
			log.Error().Err(err).Msg("dxlink session reset failed, retrying anyway")
		}
	}
	c.cleanup()

	c.mu.Lock()
	if c.ctx == nil || c.ctx.Err() != nil {
		c.ctx = ctx
	}
	c.mu.Unlock()

	connCtx, connCancel := context.WithTimeout(ctx, reconnectStepTimeout)
	defer connCancel()

	connDone := make(chan error, 1)
	go func() {
		connDone <- c.Connect(connCtx)
	}()

	select {
	case err := <-connDone:
		if err != nil {
			c.cleanup()
			return err
		}
	case <-connCtx.Done():
		c.cleanup()
		return fmt.Errorf("dxlink connect step timed out: %w", connCtx.Err())
	}

	if c.onReconnect != nil {
		reconCtx, reconCancel := context.WithTimeout(ctx, reconnectStepTimeout)
		defer reconCancel()

		reconDone := make(chan struct{})
		go func() {
			c.onReconnect(reconCtx, c)
			close(reconDone)
		}()

		select {
		case <-reconDone:
			if err := reconCtx.Err(); err != nil {
				c.cleanup()
				return fmt.Errorf("dxlink onReconnect timed out: %w", err)
			}
			if !c.Connected() {
				c.cleanup()
				return fmt.Errorf("dxlink disconnected during onReconnect")
			}
		case <-reconCtx.Done():
			c.cleanup()
			return fmt.Errorf("dxlink onReconnect timed out: %w", reconCtx.Err())
		}
	}
	return nil
}
