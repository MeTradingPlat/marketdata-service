package livecandles

import "sync"

// Broadcaster reenvia cada item nuevo a quien este suscripto a esa clave --
// generico porque el mismo mecanismo sirve para velas M1 (/ws/candles),
// sesiones intradia (/ws/snapshot) y fundamentales (/ws/fundamentals), sin
// triplicar el pub/sub. Un canal lleno descarta el item en vez de bloquear
// al publicador: un cliente WS lento nunca debe frenar al resto (guardado en
// BD, otros suscriptores).
type Broadcaster[T any] struct {
	mu   sync.RWMutex
	subs map[string]map[chan T]struct{}
}

func NewBroadcaster[T any]() *Broadcaster[T] {
	return &Broadcaster[T]{subs: make(map[string]map[chan T]struct{})}
}

const subscriberBuffer = 8

func (b *Broadcaster[T]) Subscribe(key string) (ch chan T, cancel func()) {
	ch = make(chan T, subscriberBuffer)

	b.mu.Lock()
	if b.subs[key] == nil {
		b.subs[key] = make(map[chan T]struct{})
	}
	b.subs[key][ch] = struct{}{}
	b.mu.Unlock()

	cancel = func() {
		b.mu.Lock()
		delete(b.subs[key], ch)
		if len(b.subs[key]) == 0 {
			delete(b.subs, key)
		}
		b.mu.Unlock()
		close(ch)
	}
	return ch, cancel
}

func (b *Broadcaster[T]) Publish(key string, item T) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs[key] {
		select {
		case ch <- item:
		default:
		}
	}
}
