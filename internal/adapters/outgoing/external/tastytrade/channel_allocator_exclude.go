package tastytrade

import (
	"context"
	"fmt"
)

func (a *channelAllocator) findRoomExcluding(exclude *pooledChannel) *pooledChannel {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, pc := range a.connections {
		if ch := pc.channelWithRoom(exclude); ch != nil {
			return ch
		}
	}
	return nil
}

func (a *channelAllocator) allocateExcluding(ctx context.Context, exclude *pooledChannel) (*pooledChannel, error) {
	if ch := a.findRoomExcluding(exclude); ch != nil {
		return ch, nil
	}
	a.growMu.Lock()
	defer a.growMu.Unlock()
	if ch := a.findRoomExcluding(exclude); ch != nil {
		return ch, nil
	}
	if pc := a.connectionWithRoomForChannel(); pc != nil {
		return a.openChannelOn(ctx, pc)
	}
	if a.connectionCount() >= a.maxConnections {
		return nil, fmt.Errorf("candle pool at connection ceiling (%d)", a.maxConnections)
	}
	a.staggerBeforeNewConnection(ctx)
	conn, err := a.connFactory(ctx)
	if err != nil {
		return nil, fmt.Errorf("opening new pool connection: %w", err)
	}
	pc := newPooledConnection(conn)
	if a.onReconnect != nil {
		conn.OnReconnect(func(rctx context.Context, _ *DxLinkConn) { a.onReconnect(rctx, pc) })
	}
	a.mu.Lock()
	a.connections = append(a.connections, pc)
	a.mu.Unlock()
	return a.openChannelOn(ctx, pc)
}
