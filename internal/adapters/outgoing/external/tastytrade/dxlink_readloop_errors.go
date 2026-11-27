package tastytrade

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
)

func (c *DxLinkConn) handleUnauthorizedError(ctx context.Context, env inboundEnvelope) {
	log.Error().Int64("connID", c.id).Int("channel", env.Channel).Str("message", env.Message).Msg("dxlink UNAUTHORIZED, forcing reconnect")
	if strings.Contains(env.Message, "sessions") {
		c.markSessionSaturated()
	}
	c.noteCloseReason(fmt.Sprintf("server_error: UNAUTHORIZED: %s", env.Message))
	c.retireAndReconnect(ctx)
}

func (c *DxLinkConn) handleInvalidMessageError(ctx context.Context, env inboundEnvelope) {
	log.Error().Int64("connID", c.id).Int("channel", env.Channel).Str("error", env.Error).Str("message", env.Message).Msg("dxlink INVALID_MESSAGE, forcing reconnect")
	c.noteCloseReason(fmt.Sprintf("server_error: INVALID_MESSAGE: %s", env.Message))
	c.retireAndReconnect(ctx)
}

func (c *DxLinkConn) retireAndReconnect(ctx context.Context) {
	c.mu.Lock()
	c.setAuthenticated(false)
	c.retireChannelsLocked()
	c.mu.Unlock()
	c.scheduleReconnect(ctx)
}
