package tastytrade

import "github.com/rs/zerolog/log"

func (c *DxLinkConn) handleSetupMessage() {
	if err := c.send(authMessage{Type: "AUTH", Channel: 0, Token: c.tokenFunc()}); err != nil {
		log.Error().Err(err).Int64("connID", c.id).Msg("dxlink: failed to send AUTH")
	}
}

func (c *DxLinkConn) handleAuthStateMessage(env inboundEnvelope) {
	if env.State == "AUTHORIZED" {
		c.markAuthenticated()
	}
}

func (c *DxLinkConn) handleKeepaliveMessage() {
	if err := c.send(keepaliveMessage{Type: "KEEPALIVE", Channel: 0}); err != nil {
		log.Error().Err(err).Int64("connID", c.id).Msg("dxlink: failed to reply KEEPALIVE")
	}
}

func (c *DxLinkConn) handleUnknownMessage(msgType string) {
	if c.stats.recordUnknownType(msgType) {
		log.Warn().Int64("connID", c.id).Str("type", msgType).Msg("dxlink unknown message type")
	}
}
