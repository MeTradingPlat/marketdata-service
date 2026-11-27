package tastytrade

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

func (p *CandlePool) resubscribeReconnectSymbols(ctx context.Context, symbols []string, resumeFrom map[string]time.Time) {
	p.liveMu.Lock()
	epoch := p.liveEpoch
	p.liveMu.Unlock()

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	retriesLeft := resubscribeRetryBudget
	for i, symbol := range symbols {
		if runCtx.Err() != nil {
			p.clearPendingRevive(symbols[i:])
			return
		}
		p.liveMu.Lock()
		if p.liveEpoch != epoch {
			p.liveMu.Unlock()
			p.clearPendingRevive(symbols[i:])
			return
		}
		cb := p.liveSubs[symbol]
		tick := p.liveTicks[symbol]
		p.liveMu.Unlock()
		if cb == nil {
			continue
		}
		subscribe := func() error {
			err := p.subscribeLiveAtEpoch(runCtx, symbol, resumeFrom[symbol], cb, tick, epoch)
			p.liveMu.Lock()
			changed := p.liveEpoch != epoch
			p.liveMu.Unlock()
			if changed {
				cancel()
			}
			return err
		}
		if err := resubscribeWithRetry(runCtx, subscribe, &retriesLeft, resubscribeRetryDelay); err != nil {
			if runCtx.Err() != nil {
				p.clearPendingRevive(symbols[i:])
				return
			}
			// La entrada en liveSubs es la memoria de que el simbolo debe estar
			// vivo y ReattachOrphans lo reengancha; borrarla dejaba al
			// reconciliador (100 simbolos/5 min) como unico camino.
			p.clearPendingRevive(symbols[i : i+1])
			log.Error().Err(err).Str("symbol", symbol).Msg("failed to resubscribe live candle after reconnect, leaving it for orphan heal")
		}
	}
}
