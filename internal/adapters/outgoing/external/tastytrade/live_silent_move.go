package tastytrade

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/rs/zerolog/log"
)

func (p *CandlePool) moveSilentSymbolChannel(ctx context.Context, symbol string, oldCh *pooledChannel, now time.Time) (bool, error) {
	newCh, err := p.claimLiveChannelExcluding(ctx, symbol, oldCh)
	if err != nil {
		return false, err
	}
	key := candleKey(symbol, domain.M1)
	_ = oldCh.channel.unsubscribe(symbol, domain.M1)
	_ = oldCh.channel.unsubscribeLiveDayVolume(symbol)
	oldCh.release(key)

	from := now.Add(-silentHealFallbackLookback)
	p.currentMu.Lock()
	if last, ok := p.lastClosed[symbol]; ok && !last.Timestamp.IsZero() {
		from = last.Timestamp
	}
	p.currentMu.Unlock()

	p.registerDispatch(symbol, domain.M1, "live", func(ev rawCandleEvent) { p.handleLiveEvent(symbol, ev) })
	if err := newCh.channel.subscribeLive(symbol, domain.M1, from); err != nil {
		newCh.release(key)
		return false, err
	}
	_ = newCh.channel.subscribeLiveDayVolume(symbol)
	p.noteSubscribed(symbol, now)
	p.recordSilentRepair(symbol, now)
	log.Warn().Str("symbol", symbol).Str("mode", "move_channel").Int("old_channel", oldCh.channel.id).Int("new_channel", newCh.channel.id).Msg("healed silent live symbol")
	return true, nil
}
