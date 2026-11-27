package tastytrade

import (
	"context"
	"errors"
	"time"

	"github.com/rs/zerolog/log"
)

func (p *CandlePool) moveSilentSymbolChannel(ctx context.Context, symbol string, oldCh *pooledChannel, now time.Time) (bool, error) {
	cb, _, epoch, stopping := p.readLiveSubForHeal(symbol)
	if stopping || cb == nil {
		return false, nil
	}
	from := p.knownBarTime(symbol)
	if from.IsZero() {
		from = now.Add(-silentHealFallbackLookback)
	}
	newCh, err := p.moveLiveSymbolChannel(ctx, symbol, oldCh, from, epoch, now)
	if err != nil {
		if errors.Is(err, context.Canceled) && ctx.Err() == nil {
			return false, nil
		}
		return false, err
	}
	p.recordSilentRepair(symbol, now)
	log.Warn().Str("symbol", symbol).Str("mode", "move_channel").Int("old_channel", oldCh.channel.id).Int("new_channel", newCh.channel.id).Msg("healed silent live symbol")
	return true, nil
}
