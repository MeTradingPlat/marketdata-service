package tastytrade

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// dxLinkMaxMessageBytes es el limite documentado por dxFeed para un solo
// mensaje FEED_SUBSCRIPTION (kb.dxfeed.com/en/market-data-api/dxlink.html):
// pasado esto el servidor responde INVALID_MESSAGE "content length exceeded
// 65536 bytes." -- logueamos ANTES de mandar cualquier mensaje que se
// acerque, para tener la evidencia (cuantos simbolos, que canal) la proxima
// vez que pase, en vez de reconstruirlo despues por logs indirectos.
const dxLinkMaxMessageBytes = 65536
const dxLinkWarnMessageBytes = 55000

func (c *DxLinkConn) send(v interface{}) error {
	c.mu.RLock()
	conn := c.conn
	ctx := c.ctx
	c.mu.RUnlock()
	if conn == nil {
		return fmt.Errorf("dxlink connection not open")
	}
	body, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encoding dxlink message: %w", err)
	}
	if len(body) >= dxLinkWarnMessageBytes {
		logOutgoingMessageSize(v, len(body))
	}
	c.writeMu.Lock()
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	err = conn.WriteMessage(websocket.TextMessage, body)
	c.writeMu.Unlock()
	if err != nil {
		c.noteCloseReason(fmt.Sprintf("send_error: %v", err))
		if ctx == nil {
			ctx = context.Background()
		}
		c.dropAndReconnect(ctx)
		return err
	}
	if msg, ok := v.(feedSubscriptionMessage); ok {
		dxLinkSubChanges.record(len(msg.Add), len(msg.Remove))
	}
	return nil
}

func logOutgoingMessageSize(v interface{}, bytes int) {
	event := log.Warn().Int("bytes", bytes).Int("limit", dxLinkMaxMessageBytes)
	if msg, ok := v.(feedSubscriptionMessage); ok {
		event = event.Int("channel", msg.Channel).Int("add", len(msg.Add)).Int("remove", len(msg.Remove))
	}
	event.Msg("dxlink outgoing message near or over the documented size limit")
}

func (c *DxLinkConn) OpenChannel(ctx context.Context) (*dxLinkChannel, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !c.Connected() {
		return nil, fmt.Errorf("dxlink connection not authenticated")
	}
	c.mu.Lock()
	c.nextChannelID += 2
	id := int(c.nextChannelID)
	ch := newDxLinkChannel(id, c)
	c.channels[id] = ch
	c.mu.Unlock()

	c.stats.opensRequested.Add(1)
	if err := ch.open(ctx); err != nil {
		if c.Connected() && !ch.retired.Load() {
			if closeErr := ch.close(); closeErr == nil {
				c.stats.cancelsSent.Add(1)
			}
		}
		c.stats.recordAbandonedChannel(id, time.Now())
		c.mu.Lock()
		delete(c.channels, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("opening dxlink channel: %w", err)
	}
	return ch, nil
}

func (c *DxLinkConn) CloseChannel(ch *dxLinkChannel) {
	_ = ch.close()
	c.mu.Lock()
	delete(c.channels, ch.id)
	c.mu.Unlock()
}

func (c *DxLinkConn) channel(id int) *dxLinkChannel {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.channels[id]
}
