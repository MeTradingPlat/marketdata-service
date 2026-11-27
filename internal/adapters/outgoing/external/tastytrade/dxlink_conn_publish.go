package tastytrade

import (
	"context"
	"fmt"
	"time"

	"github.com/gorilla/websocket"
)

func (c *DxLinkConn) publishConn(ctx context.Context, conn *websocket.Conn) (context.Context, chan error, error) {
	handshakeDone := make(chan error, 1)
	c.mu.Lock()
	if c.closing.Load() {
		c.mu.Unlock()
		_ = conn.Close()
		return nil, nil, fmt.Errorf("dxlink connection closing")
	}
	if c.ctx == nil || c.ctx.Err() != nil {
		c.ctx = context.WithoutCancel(ctx)
	}
	runCtx := c.ctx
	c.clearCloseReason()
	c.stats.reset()
	c.socketOpenedAtNano.Store(time.Now().UnixNano())
	c.conn = conn
	c.setAuthenticated(false)
	c.handshakeDone = handshakeDone
	c.retireChannelsLocked()
	c.channels = make(map[int]*dxLinkChannel)
	c.connDone = make(chan struct{})
	c.mu.Unlock()
	c.touchLastMessage()
	return runCtx, handshakeDone, nil
}
