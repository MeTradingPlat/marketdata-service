package tastytrade

import (
	"context"
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/rs/zerolog/log"
)

// SubscribeLive es la unica suscripcion M1 de un simbolo: se abre con
// FromTime = from (para retomar exactamente donde quedo el ultimo dato
// guardado) y se queda abierta para siempre, sin un FetchHistory M1
// separado antes ni un remove/add de por medio -- ver el comentario de
// subscribeLive sobre por que esa secuencia dejaba el streaming mudo.
func (p *CandlePool) SubscribeLive(ctx context.Context, symbol string, from time.Time, onClosed func(domain.Candle), onTick func(domain.Candle)) error {
	return p.subscribeLiveInternal(ctx, symbol, from, onClosed, onTick, false, 0)
}

func (p *CandlePool) subscribeLiveAtEpoch(ctx context.Context, symbol string, from time.Time, onClosed func(domain.Candle), onTick func(domain.Candle), epoch uint64) error {
	return p.subscribeLiveInternal(ctx, symbol, from, onClosed, onTick, true, epoch)
}

func (p *CandlePool) readLiveSubForHeal(sym string) (func(domain.Candle), func(domain.Candle), uint64, bool) {
	p.liveMu.Lock()
	defer p.liveMu.Unlock()
	if p.liveStopping > 0 {
		return nil, nil, 0, true
	}
	return p.liveSubs[sym], p.liveTicks[sym], p.liveEpoch, false
}

func (p *CandlePool) registerLiveSub(symbol string, onClosed func(domain.Candle), onTick func(domain.Candle), checkEpoch bool, epoch uint64) (func(domain.Candle), bool, bool) {
	p.liveMu.Lock()
	defer p.liveMu.Unlock()
	if checkEpoch && (p.liveStopping > 0 || p.liveEpoch != epoch) {
		return nil, false, false
	}
	prevTick, hadTick := p.liveTicks[symbol]
	p.liveSubs[symbol] = onClosed
	p.liveTicks[symbol] = onTick
	return prevTick, hadTick, true
}

func (p *CandlePool) subscribeLiveInternal(ctx context.Context, symbol string, from time.Time, onClosed func(domain.Candle), onTick func(domain.Candle), checkEpoch bool, epoch uint64) error {
	if p.liveChannelFor(symbol) != nil && p.hasLiveDispatch(symbol) {
		return nil
	}
	from = p.resolveResume(symbol, from, time.Now())
	ch, err := p.claimLiveChannel(ctx, symbol)
	if err != nil {
		return fmt.Errorf("allocating channel for %s live M1: %w", symbol, err)
	}

	prevTick, hadTick, ok := p.registerLiveSub(symbol, onClosed, onTick, checkEpoch, epoch)
	if !ok {
		ch.release(candleKey(symbol, domain.M1))
		return context.Canceled
	}

	dispatchID := p.registerDispatch(symbol, domain.M1, "live", func(ev rawCandleEvent) { p.handleLiveEvent(symbol, ev) })

	if err := ch.channel.subscribeLive(symbol, domain.M1, from); err != nil {
		p.liveMu.Lock()
		if hadTick {
			p.liveTicks[symbol] = prevTick
		} else {
			delete(p.liveTicks, symbol)
		}
		p.liveMu.Unlock()
		p.unregisterDispatchIfCurrent(symbol, domain.M1, dispatchID, "live")
		ch.release(candleKey(symbol, domain.M1))
		return fmt.Errorf("subscribing live M1 for %s: %w", symbol, err)
	}
	p.noteSubscribed(symbol, time.Now())
	if err := ch.channel.subscribeLiveDayVolume(symbol); err != nil {
		log.Warn().Err(err).Str("symbol", symbol).Msg("failed to subscribe live day volume (Trade/TradeETH), falling back to batch fetch")
	}
	return nil
}
