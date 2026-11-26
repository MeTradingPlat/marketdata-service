package tastytrade

import (
	"context"

	"github.com/rs/zerolog/log"
)

func (g *Gateway) openInterestFailure(ctx context.Context, symbol, msg string, err error) (float64, bool, error) {
	log.Debug().Err(err).Str("symbol", symbol).Msg(msg)
	if ctx.Err() == nil {
		g.openInterestCache.putError(symbol, err)
	}
	return 0, false, err
}
