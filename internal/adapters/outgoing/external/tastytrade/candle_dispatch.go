package tastytrade

import (
	"sync/atomic"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/rs/zerolog/log"
)

type dispatchEntry struct {
	id      uint64
	source  string
	handler func(rawCandleEvent)
}

func (p *CandlePool) wireChannel(ch *dxLinkChannel) {
	ch.setOnCandle(p.routeEvent)
	ch.setOnTrade(p.handleLiveTradeVolume)
	ch.setOnTradeETH(p.handleLiveTradeETHVolume)
}

func (p *CandlePool) routeEvent(ev rawCandleEvent) {
	symbol, tf, ok := parseWireSymbol(ev.Symbol)
	if !ok {
		return
	}
	key := candleKey(symbol, tf)
	p.noteRouted(symbol, time.Now())
	p.dispatchMu.RLock()
	entry, found := p.dispatch[key]
	p.dispatchMu.RUnlock()
	if found {
		entry.handler(ev)
		return
	}
	if p.retired.wasRecentlyRetired(key, time.Now()) {
		if n := atomic.AddInt64(&p.lateEvents, 1); n%1000 == 1 {
			log.Debug().Str("symbol", symbol).Str("timeframe", string(tf)).Int64("total_late_events", n).
				Msg("late candle event after handler retired")
		}
		return
	}
	if n := atomic.AddInt64(&p.orphanEvents, 1); n%20 == 1 {
		log.Warn().Str("symbol", symbol).Str("timeframe", string(tf)).Int64("total_orphan_events", n).
			Msg("candle event with no registered handler -- possible leaked subscription")
	}
}

// registerDispatch devuelve un id de esta registracion en particular --
// unregisterDispatchIfCurrent lo necesita para no borrar por accidente el
// dispatch de una registracion MAS NUEVA para la misma clave (ver
// unsubscribeDrainPeriod: el borrado se agenda con retraso). source es
// puramente diagnostico (queda en el log si esta registracion pisa una
// existente) -- confirmar EXACTAMENTE que esta pisando a que fue dificil de
// ver solo con volcados de goroutines.
func (p *CandlePool) registerDispatch(symbol string, tf domain.Timeframe, source string, handler func(rawCandleEvent)) uint64 {
	p.dispatchMu.Lock()
	defer p.dispatchMu.Unlock()
	key := candleKey(symbol, tf)
	prev, replaced := p.dispatch[key]
	if replaced && prev.source == "live" && source != "live" {
		log.Warn().Str("symbol", symbol).Str("timeframe", string(tf)).Str("source", source).
			Uint64("live_id", prev.id).
			Msg("refusing to overwrite live dispatch entry")
		return 0
	}
	p.dispatchSeq++
	id := p.dispatchSeq
	if replaced {
		log.Info().Str("symbol", symbol).Str("timeframe", string(tf)).Str("source", source).
			Uint64("new_id", id).Uint64("replaced_id", prev.id).
			Msg("dispatch registration replaced an existing entry")
	}
	p.dispatch[key] = dispatchEntry{id: id, source: source, handler: handler}
	return id
}

func (p *CandlePool) unregisterDispatchIfCurrent(symbol string, tf domain.Timeframe, id uint64, source string) {
	if id == 0 {
		return
	}
	p.dispatchMu.Lock()
	defer p.dispatchMu.Unlock()
	key := candleKey(symbol, tf)
	if entry, ok := p.dispatch[key]; ok && entry.id == id {
		if entry.source == "live" && source != "live" {
			return
		}
		delete(p.dispatch, key)
		p.retired.mark(key, time.Now())
		log.Info().Str("symbol", symbol).Str("timeframe", string(tf)).Str("source", source).Uint64("id", id).
			Msg("dispatch entry removed")
	}
}

func (p *CandlePool) hasLiveDispatch(symbol string) bool {
	p.dispatchMu.RLock()
	defer p.dispatchMu.RUnlock()
	entry, ok := p.dispatch[candleKey(symbol, domain.M1)]
	return ok && entry.source == "live"
}
