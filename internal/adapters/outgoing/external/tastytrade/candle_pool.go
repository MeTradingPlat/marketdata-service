package tastytrade

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/rs/zerolog/log"
)

const historyDefaultWait = 15 * time.Second

// CandlePool reparte simbolos entre canales/conexiones DxLink pooled (via
// channelAllocator) y enruta cada evento crudo que llega a la suscripcion
// (en vivo o historial) que le corresponde -- un canal pooled sirve varias
// suscripciones distintas a la vez, asi que el dispatch es un registro
// central por simbolo+temporalidad, no un callback fijo por canal.
type CandlePool struct {
	allocator *channelAllocator

	dispatchMu sync.RWMutex
	dispatch   map[string]func(rawCandleEvent)

	liveMu   sync.Mutex
	liveSubs map[string]func(domain.Candle)

	currentMu sync.Mutex
	current   map[string]domain.Candle

	// orphanEvents cuenta eventos que llegan para un simbolo+temporalidad
	// que ya no tiene handler registrado -- si esto crece con el tiempo
	// despues de que un fetch de historial deberia haberse desuscrito,
	// confirma una fuga de suscripcion server-side. Diagnostico temporal.
	orphanEvents int64
}

func NewCandlePool(connFactory func(ctx context.Context) (*DxLinkConn, error), maxConnections int) *CandlePool {
	p := &CandlePool{
		dispatch: make(map[string]func(rawCandleEvent)),
		liveSubs: make(map[string]func(domain.Candle)),
		current:  make(map[string]domain.Candle),
	}
	p.allocator = newChannelAllocator(connFactory, p.wireChannel, p.handleConnectionReconnect, maxConnections)
	return p
}

func (p *CandlePool) Connected() bool {
	connected, _ := p.allocator.stats()
	return connected > 0
}

// WarmUp abre la primera conexion/canal por adelantado -- sin esto, la
// primera peticion real (o el health check) paga el costo del handshake
// DxLink completo, y un fallo de credenciales/red recien se descubre en
// produccion sirviendo trafico en vez de al arrancar.
func (p *CandlePool) WarmUp(ctx context.Context) error {
	_, err := p.allocator.allocate(ctx)
	return err
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
	handler := p.dispatch[candleKey(symbol, tf)]
	p.dispatchMu.RUnlock()
	if handler != nil {
		handler(ev)
		return
	}
	if n := atomic.AddInt64(&p.orphanEvents, 1); n%20 == 1 {
		log.Warn().Str("symbol", symbol).Str("timeframe", string(tf)).Int64("total_orphan_events", n).
			Msg("candle event with no registered handler -- possible leaked subscription")
	}
}

func (p *CandlePool) registerDispatch(symbol string, tf domain.Timeframe, handler func(rawCandleEvent)) {
	p.dispatchMu.Lock()
	p.dispatch[candleKey(symbol, tf)] = handler
	p.dispatchMu.Unlock()
}

func (p *CandlePool) unregisterDispatch(symbol string, tf domain.Timeframe) {
	p.dispatchMu.Lock()
	delete(p.dispatch, candleKey(symbol, tf))
	p.dispatchMu.Unlock()
}

// SubscribeLive y FetchHistory(..., M1, ...) comparten la misma clave de
// dispatch para un simbolo -- si se llaman a la vez para el mismo simbolo,
// el que limpie al final se lleva la suscripcion del otro. El flujo
// correcto es backfill primero, en vivo despues (ver IngestCandlesService),
// nunca concurrentes para el mismo simbolo+M1.
func (p *CandlePool) SubscribeLive(ctx context.Context, symbol string, onClosed func(domain.Candle)) error {
	ch, err := p.allocator.allocate(ctx)
	if err != nil {
		return fmt.Errorf("allocating channel for %s live M1: %w", symbol, err)
	}

	p.liveMu.Lock()
	p.liveSubs[symbol] = onClosed
	p.liveMu.Unlock()

	p.registerDispatch(symbol, domain.M1, func(ev rawCandleEvent) { p.handleLiveEvent(symbol, ev) })
	ch.occupy(candleKey(symbol, domain.M1))

	if err := ch.channel.subscribeLive(symbol, domain.M1); err != nil {
		return fmt.Errorf("subscribing live M1 for %s: %w", symbol, err)
	}
	return nil
}

// handleLiveEvent detecta el cierre de una vela: mientras los eventos que
// llegan comparten el mismo timestamp, son actualizaciones de la vela en
// formacion; un timestamp nuevo significa que la anterior ya cerro.
func (p *CandlePool) handleLiveEvent(symbol string, ev rawCandleEvent) {
	p.currentMu.Lock()
	prev, exists := p.current[symbol]
	if exists && !prev.Timestamp.Equal(ev.Timestamp) {
		closed := prev
		p.current[symbol] = mergeCandle(domain.Candle{}, ev, symbol, domain.M1)
		p.currentMu.Unlock()
		p.dispatchClosed(symbol, closed)
		return
	}
	p.current[symbol] = mergeCandle(prev, ev, symbol, domain.M1)
	p.currentMu.Unlock()
}

func (p *CandlePool) dispatchClosed(symbol string, c domain.Candle) {
	if !c.IsComplete() {
		return
	}
	p.liveMu.Lock()
	cb := p.liveSubs[symbol]
	p.liveMu.Unlock()
	if cb != nil {
		cb(c)
	}
}

// handleConnectionReconnect corre cuando UNA conexion del pool se
// reconecta -- sus canales viejos ya no sirven (IDs de un socket que ya no
// existe), pero la conexion en si sigue siendo la misma valida, asi que se
// resetea (no se saca del pool) y se vuelve a pedir un slot para cada
// simbolo en vivo que tenia.
func (p *CandlePool) handleConnectionReconnect(ctx context.Context, pc *pooledConnection) {
	symbols := pc.liveSymbols()
	pc.reset()
	for _, symbol := range symbols {
		p.liveMu.Lock()
		cb := p.liveSubs[symbol]
		p.liveMu.Unlock()
		if cb == nil {
			continue
		}
		if err := p.SubscribeLive(ctx, symbol, cb); err != nil {
			log.Error().Err(err).Str("symbol", symbol).Msg("failed to resubscribe live candle after reconnect")
		}
	}
}

func (p *CandlePool) FetchHistory(ctx context.Context, symbol string, tf domain.Timeframe, from time.Time) ([]domain.Candle, error) {
	ch, err := p.allocator.allocate(ctx)
	if err != nil {
		return nil, fmt.Errorf("allocating channel for %s %s history: %w", symbol, tf, err)
	}

	key := candleKey(symbol, tf)
	ch.occupy(key)
	defer func() {
		ch.release(key)
		p.unregisterDispatch(symbol, tf)
		_ = ch.channel.unsubscribe(symbol, tf)
	}()

	collector := newHistoryCollector(symbol, tf)
	p.registerDispatch(symbol, tf, collector.onCandle)

	if err := ch.channel.subscribeHistory(symbol, tf, from); err != nil {
		return nil, fmt.Errorf("subscribing history: %w", err)
	}
	if err := waitForData(ctx, collector.settled, historyDefaultWait); err != nil {
		return nil, err
	}
	return collector.complete(), nil
}
