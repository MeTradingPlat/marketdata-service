package tastytrade

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

const historyDefaultWait = 15 * time.Second

// CandlePool maneja UNA conexion DxLink persistente para suscripciones en
// vivo (M1 solamente, ver plan de arquitectura) mas canales efimeros para
// pedidos de historial. Sondear multiples conexiones/particionar simbolos
// entre ellas es una optimizacion de escala futura -- probar el flujo
// completo funcionando de punta a punta con una sola conexion viene primero.
type CandlePool struct {
	conn *DxLinkConn

	liveMu      sync.Mutex
	liveChannel *dxLinkChannel
	liveSubs    map[string]func(domain.Candle)

	currentMu sync.Mutex
	current   map[string]domain.Candle
}

func NewCandlePool(conn *DxLinkConn) *CandlePool {
	p := &CandlePool{
		conn:     conn,
		liveSubs: make(map[string]func(domain.Candle)),
		current:  make(map[string]domain.Candle),
	}
	conn.OnReconnect(p.onReconnect)
	return p
}

func (p *CandlePool) SubscribeLive(ctx context.Context, symbol string, onClosed func(domain.Candle)) error {
	ch, err := p.ensureLiveChannel(ctx)
	if err != nil {
		return err
	}
	p.liveMu.Lock()
	p.liveSubs[symbol] = onClosed
	p.liveMu.Unlock()
	return ch.subscribeLive(symbol, domain.M1)
}

func (p *CandlePool) ensureLiveChannel(ctx context.Context) (*dxLinkChannel, error) {
	p.liveMu.Lock()
	defer p.liveMu.Unlock()
	if p.liveChannel != nil {
		return p.liveChannel, nil
	}
	ch, err := p.conn.OpenChannel(ctx)
	if err != nil {
		return nil, fmt.Errorf("opening live candle channel: %w", err)
	}
	ch.setOnCandle(p.handleLiveEvent)
	p.liveChannel = ch
	return ch, nil
}

// handleLiveEvent detecta el cierre de una vela: mientras los eventos que
// llegan comparten el mismo timestamp, son actualizaciones de la vela en
// formacion; un timestamp nuevo significa que la anterior ya cerro.
func (p *CandlePool) handleLiveEvent(ev rawCandleEvent) {
	symbol := stripCandleSuffix(ev.Symbol)

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

func (p *CandlePool) onReconnect(ctx context.Context, _ *DxLinkConn) {
	p.liveMu.Lock()
	p.liveChannel = nil
	symbols := make([]string, 0, len(p.liveSubs))
	for s := range p.liveSubs {
		symbols = append(symbols, s)
	}
	p.liveMu.Unlock()

	ch, err := p.ensureLiveChannel(ctx)
	if err != nil {
		return
	}
	for _, s := range symbols {
		_ = ch.subscribeLive(s, domain.M1)
	}
}

func (p *CandlePool) FetchHistory(ctx context.Context, symbol string, tf domain.Timeframe, from time.Time) ([]domain.Candle, error) {
	ch, err := p.conn.OpenChannel(ctx)
	if err != nil {
		return nil, fmt.Errorf("opening history channel: %w", err)
	}
	defer p.conn.CloseChannel(ch)

	collector := newHistoryCollector(symbol, tf)
	ch.setOnCandle(collector.onCandle)

	if err := ch.subscribeHistory(symbol, tf, from); err != nil {
		return nil, fmt.Errorf("subscribing history: %w", err)
	}
	if err := waitForData(ctx, collector.settled, historyDefaultWait); err != nil {
		return nil, err
	}
	return collector.complete(), nil
}
