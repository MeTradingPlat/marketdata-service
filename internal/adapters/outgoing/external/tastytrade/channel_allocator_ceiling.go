package tastytrade

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
)

func (a *channelAllocator) allocateAtCeiling(ctx context.Context, exclude *pooledChannel, keys ...string) (*pooledChannel, error) {
	if ch := a.findRoomExcludingFiltered(exclude, true, keys...); ch != nil {
		return ch, nil
	}
	if pc := a.connectionWithRoomForChannelFiltered(true); pc != nil {
		return a.openChannelOn(ctx, pc, keys...)
	}
	return nil, fmt.Errorf("candle pool at connection ceiling (%d)", a.maxConnections)
}

func (a *channelAllocator) fallbackToQuarantinedOnFactoryError(ctx context.Context, exclude *pooledChannel, factoryErr error, keys ...string) (*pooledChannel, error) {
	if ctx.Err() == nil {
		if ch, qErr := a.allocateAtCeiling(ctx, exclude, keys...); qErr == nil {
			log.Warn().Err(factoryErr).Msg("dxlink new connection failed, reusing quarantined connection")
			return ch, nil
		}
	}
	return nil, fmt.Errorf("opening new pool connection: %w", factoryErr)
}
