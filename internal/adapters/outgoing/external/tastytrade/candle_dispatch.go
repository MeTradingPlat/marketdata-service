package tastytrade

import (
	"sync/atomic"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/rs/zerolog/log"
)

type dispatchEntry struct {
	id      uint64
	handler func(rawCandleEvent)
}

func (p *CandlePool) wireChannel(ch *dxLinkChannel) {
	ch.setOnCandle(p.routeEvent)
}

func (p *CandlePool) routeEvent(ev rawCandleEvent) {
	symbol, tf, ok := parseWireSymbol(ev.Symbol)
	if !ok {
		return
	}
	p.dispatchMu.RLock()
	entry, found := p.dispatch[candleKey(symbol, tf)]
	p.dispatchMu.RUnlock()
	if found {
		entry.handler(ev)
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
	p.dispatchSeq++
	id := p.dispatchSeq
	key := candleKey(symbol, tf)
	if prev, ok := p.dispatch[key]; ok {
		log.Info().Str("symbol", symbol).Str("timeframe", string(tf)).Str("source", source).
			Uint64("new_id", id).Uint64("replaced_id", prev.id).
			Msg("dispatch registration replaced an existing entry")
	}
	p.dispatch[key] = dispatchEntry{id: id, handler: handler}
	return id
}

func (p *CandlePool) unregisterDispatchIfCurrent(symbol string, tf domain.Timeframe, id uint64, source string) {
	p.dispatchMu.Lock()
	defer p.dispatchMu.Unlock()
	key := candleKey(symbol, tf)
	if entry, ok := p.dispatch[key]; ok && entry.id == id {
		delete(p.dispatch, key)
		log.Info().Str("symbol", symbol).Str("timeframe", string(tf)).Str("source", source).Uint64("id", id).
			Msg("dispatch entry removed")
	}
}
