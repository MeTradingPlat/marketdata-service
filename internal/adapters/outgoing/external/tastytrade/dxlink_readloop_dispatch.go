package tastytrade

import "context"

func (c *DxLinkConn) dispatchChannelMessage(env inboundEnvelope) bool {
	switch env.Type {
	case "CHANNEL_OPENED":
		c.handleChannelOpenedMessage(env)
	case "FEED_CONFIG":
		c.handleFeedConfigMessage(env)
	case "FEED_DATA":
		c.handleFeedDataMessage(env)
	case "CHANNEL_CLOSED":
		c.handleChannelClosedMessage(env)
	default:
		return false
	}
	return true
}

func (c *DxLinkConn) dispatchControlMessage(ctx context.Context, env inboundEnvelope) bool {
	switch env.Type {
	case "SETUP":
		c.handleSetupMessage()
	case "AUTH_STATE":
		c.handleAuthStateMessage(env)
	case "KEEPALIVE":
		c.handleKeepaliveMessage()
	case "ERROR":
		c.handleError(ctx, env)
	default:
		return false
	}
	return true
}
