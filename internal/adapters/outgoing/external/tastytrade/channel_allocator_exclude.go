package tastytrade

import (
	"context"
	"time"
)

func (a *channelAllocator) findRoomExcluding(exclude *pooledChannel, keys ...string) *pooledChannel {
	return a.findRoomExcludingFiltered(exclude, false, keys...)
}

func (a *channelAllocator) findRoomExcludingFiltered(exclude *pooledChannel, allowQuarantined bool, keys ...string) *pooledChannel {
	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	if allowQuarantined {
		return a.findBestQuarantinedRoomLocked(exclude, keys...)
	}
	for _, pc := range a.connections {
		if pc.isQuarantined(now) {
			continue
		}
		if ch := pc.channelWithRoom(exclude, keys...); ch != nil {
			return ch
		}
	}
	return nil
}

func (a *channelAllocator) findBestQuarantinedRoomLocked(exclude *pooledChannel, keys ...string) *pooledChannel {
	var best *pooledChannel
	var bestStrikes int
	var bestUntil time.Time
	for _, pc := range a.connections {
		ch := pc.channelFitting(exclude, keys...)
		if ch == nil {
			continue
		}
		strikes, until := pc.quarantineRank()
		if best == nil || lessQuarantineRank(strikes, until, bestStrikes, bestUntil) {
			best, bestStrikes, bestUntil = ch, strikes, until
		}
	}
	if best != nil && best.reserve(keys...) {
		return best
	}
	return nil
}

func (a *channelAllocator) allocateExcluding(ctx context.Context, exclude *pooledChannel, keys ...string) (*pooledChannel, error) {
	if err := validateReserveKeys(keys); err != nil {
		return nil, err
	}
	if ch := a.findRoomExcluding(exclude, keys...); ch != nil {
		return ch, nil
	}
	if err := a.lockGrow(ctx); err != nil {
		return nil, err
	}
	defer a.unlockGrow()
	if ch := a.findRoomExcluding(exclude, keys...); ch != nil {
		return ch, nil
	}
	if pc := a.connectionWithRoomForChannel(); pc != nil {
		return a.openChannelOn(ctx, pc, keys...)
	}
	if a.connectionCount() >= a.maxConnections {
		return a.allocateAtCeiling(ctx, exclude, keys...)
	}
	a.staggerBeforeNewConnection(ctx)
	conn, err := a.connFactory(ctx)
	if err != nil {
		return a.fallbackToQuarantinedOnFactoryError(ctx, exclude, err, keys...)
	}
	pc := newPooledConnection(conn)
	if a.onReconnect != nil {
		conn.OnReconnect(func(rctx context.Context, _ *DxLinkConn) { a.onReconnect(rctx, pc) })
	}
	a.mu.Lock()
	a.connections = append(a.connections, pc)
	a.mu.Unlock()
	return a.openChannelOn(ctx, pc, keys...)
}
