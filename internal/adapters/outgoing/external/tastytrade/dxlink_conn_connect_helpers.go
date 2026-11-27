package tastytrade

import (
	"context"
	"fmt"
	"time"
)

func (c *DxLinkConn) withCloseCancel(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.closedCh == nil {
		return context.WithCancel(ctx)
	}
	derived, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-c.closedCh:
			cancel()
		case <-derived.Done():
		}
	}()
	return derived, cancel
}

func (c *DxLinkConn) waitHandshake(ctx context.Context, handshakeDone <-chan error) error {
	select {
	case err := <-handshakeDone:
		if err != nil {
			return err
		}
	case <-time.After(handshakeTimeout):
		// Sin este cleanup, el socket recien abierto (AUTH nunca llego a
		// AUTHORIZED) y su readLoop quedan colgando sin ningun vigia: el
		// healthCheckLoop que detecta conexiones zombie solo arranca DESPUES
		// de un handshake exitoso (en Connect), asi que nadie mas los cierra.
		c.noteCloseReason("handshake_timeout")
		c.cleanup()
		return fmt.Errorf("dxlink handshake timed out")
	case <-c.closedCh:
		c.noteCloseReason("handshake_closed")
		c.cleanup()
		return fmt.Errorf("dxlink connection closed during handshake")
	case <-ctx.Done():
		c.noteCloseReason(fmt.Sprintf("handshake_canceled: %v", ctx.Err()))
		c.cleanup()
		return ctx.Err()
	}
	if c.closing.Load() {
		c.noteCloseReason("handshake_closing")
		c.cleanup()
		return fmt.Errorf("dxlink connection closing")
	}
	return nil
}
