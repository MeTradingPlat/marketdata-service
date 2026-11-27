package tastytrade

import (
	"context"
	"time"
)

func (a *channelAllocator) findRoomExcluding(exclude *pooledChannel) *pooledChannel {
	return a.findRoomExcludingFiltered(exclude, false)
}

func (a *channelAllocator) findRoomExcludingFiltered(exclude *pooledChannel, allowQuarantined bool) *pooledChannel {
	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	var best *pooledChannel
	var bestStrikes int
	var bestUntil time.Time
	for _, pc := range a.connections {
		if !allowQuarantined && pc.isQuarantined(now) {
			continue
		}
		ch := pc.channelWithRoom(exclude)
		if ch == nil {
			continue
		}
		if !allowQuarantined {
			return ch
		}
		strikes, until := pc.quarantineRank()
		if best == nil || lessQuarantineRank(strikes, until, bestStrikes, bestUntil) {
			best, bestStrikes, bestUntil = ch, strikes, until
		}
	}
	return best
}

func (a *channelAllocator) allocateExcluding(ctx context.Context, exclude *pooledChannel) (*pooledChannel, error) {
	if ch := a.findRoomExcluding(exclude); ch != nil {
		return ch, nil
	}
	if err := a.lockGrow(ctx); err != nil {
		return nil, err
	}
	defer a.unlockGrow()
	if ch := a.findRoomExcluding(exclude); ch != nil {
		return ch, nil
	}
	if pc := a.connectionWithRoomForChannel(); pc != nil {
		return a.openChannelOn(ctx, pc)
	}
	if a.connectionCount() >= a.maxConnections {
		return a.allocateAtCeiling(ctx, exclude)
	}
	a.staggerBeforeNewConnection(ctx)
	conn, err := a.connFactory(ctx)
	if err != nil {
		return a.fallbackToQuarantinedOnFactoryError(ctx, exclude, err)
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
