package tastytrade

import (
	"errors"
	"fmt"
	"time"

	"github.com/gorilla/websocket"
)

func (c *DxLinkConn) noteCloseReason(reason string) {
	if reason == "" {
		return
	}
	c.closeReasonMu.Lock()
	if c.closeReason == "" {
		c.closeReason = reason
	}
	c.closeReasonMu.Unlock()
}

func (c *DxLinkConn) clearCloseReason() {
	c.closeReasonMu.Lock()
	c.closeReason = ""
	c.closeReasonMu.Unlock()
}

func (c *DxLinkConn) closeReasonText() string {
	return lockedString(&c.closeReasonMu, &c.closeReason)
}

func (c *DxLinkConn) lastMessageAt() time.Time {
	return unixTimeFromNano(c.lastMessageAtUnixNano.Load())
}

func (c *DxLinkConn) socketLifetime() time.Duration {
	return ageSinceUnixNano(c.socketOpenedAtNano.Load())
}

func formatReadErrorReason(err error) string {
	var ce *websocket.CloseError
	if errors.As(err, &ce) {
		return fmt.Sprintf("read_error: code=%d text=%s err=%v", ce.Code, ce.Text, err)
	}
	return fmt.Sprintf("read_error: %v", err)
}
