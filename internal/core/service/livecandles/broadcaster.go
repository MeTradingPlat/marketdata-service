package livecandles

import (
	"sync"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// Broadcaster reenvia cada vela M1 recien cerrada a quien este suscripto a
// ese simbolo -- el puente entre StreamLive (que ya recibe cada vela en vivo
// para guardarla en BD) y el WS de /ws/candles (que necesita la misma vela
// sin tocar la BD de nuevo). Un canal lleno descarta la vela en vez de
// bloquear al publicador: un cliente WS lento nunca debe frenar el guardado
// real en Postgres.
type Broadcaster struct {
	mu   sync.RWMutex
	subs map[string]map[chan domain.Candle]struct{}
}

func NewBroadcaster() *Broadcaster {
	return &Broadcaster{subs: make(map[string]map[chan domain.Candle]struct{})}
}

const subscriberBuffer = 8

func (b *Broadcaster) Subscribe(symbol string) (ch chan domain.Candle, cancel func()) {
	ch = make(chan domain.Candle, subscriberBuffer)

	b.mu.Lock()
	if b.subs[symbol] == nil {
		b.subs[symbol] = make(map[chan domain.Candle]struct{})
	}
	b.subs[symbol][ch] = struct{}{}
	b.mu.Unlock()

	cancel = func() {
		b.mu.Lock()
		delete(b.subs[symbol], ch)
		if len(b.subs[symbol]) == 0 {
			delete(b.subs, symbol)
		}
		b.mu.Unlock()
		close(ch)
	}
	return ch, cancel
}

func (b *Broadcaster) Publish(c domain.Candle) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs[c.Symbol] {
		select {
		case ch <- c:
		default:
		}
	}
}
