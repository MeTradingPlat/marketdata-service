package tastytrade

import (
	"time"

	"github.com/rs/zerolog/log"
)

func (c *DxLinkConn) handleChannelOpenedMessage(env inboundEnvelope) {
	if env.Service != "FEED" {
		return
	}
	if ch := c.channel(env.Channel); ch != nil {
		if err := ch.handleOpened(); err != nil {
			log.Error().Err(err).Int64("connID", c.id).Int("channel", env.Channel).Msg("dxlink: failed to send FEED_SETUP")
		}
		return
	}
	c.logLateResponse(env.Type, env.Channel)
}

func (c *DxLinkConn) handleFeedConfigMessage(env inboundEnvelope) {
	if ch := c.channel(env.Channel); ch != nil {
		ch.handleConfigured()
		return
	}
	c.logLateResponse(env.Type, env.Channel)
}

func (c *DxLinkConn) handleFeedDataMessage(env inboundEnvelope) {
	if ch := c.channel(env.Channel); ch != nil {
		ch.handleData(env.Data)
	}
}

func (c *DxLinkConn) handleChannelClosedMessage(env inboundEnvelope) {
	c.stats.channelsClosed.Add(1)
	if c.channel(env.Channel) != nil {
		log.Warn().Int64("connID", c.id).Int("channel", env.Channel).Msg("dxlink server closed active channel")
	}
}

func (c *DxLinkConn) logLateResponse(msgType string, channel int) {
	c.stats.lateResponses.Add(1)
	ev := log.Warn().Int64("connID", c.id).Int("channel", channel).Str("type", msgType)
	if age, ok := c.stats.abandonedAge(channel, time.Now()); ok {
		ev = ev.Dur("abandoned_age", age)
	}
	ev.Msg("dxlink late response on unregistered channel")
}
