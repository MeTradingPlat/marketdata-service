package livecandles

import "sync"

// Broadcaster reenvia cada item nuevo a cada callback registrado para esa
// clave -- generico porque el mismo mecanismo sirve para velas M1
// (/ws/candles), sesiones intradia (/ws/snapshot) y fundamentales
// (/ws/fundamentals), sin triplicar el pub/sub.
//
// push corre SINCRONICAMENTE en la goroutine de quien llama a Publish --
// debe ser rapido y no bloqueante (el patron esperado es encolar en un canal
// PROPIO del suscriptor con select/default, nunca escribir un socket
// directo desde aca). Reemplaza el diseño anterior de un canal + una
// goroutine dedicada por suscriptor: con miles de suscripciones activas a
// la vez (ej. un escaner sin pre-filtros evaluando en tiempo real el
// universo completo) esas goroutines quedaban paradas para siempre
// esperando datos, y esa cantidad -- no el volumen de velas en si -- era lo
// que tumbaba el contenedor por OOM (confirmado en vivo el 2026-09-17: de
// 86 a 9000+ goroutines en minutos). Sin canal ni goroutine por
// suscripcion, la cantidad de suscriptores deja de costar memoria de forma
// lineal.
type Broadcaster[T any] struct {
	mu   sync.RWMutex
	subs map[string]map[int64]func(T)
	next int64
}

func NewBroadcaster[T any]() *Broadcaster[T] {
	return &Broadcaster[T]{subs: make(map[string]map[int64]func(T))}
}

// Subscribe registra push para reenvios de esa key hasta que se llame
// cancel.
func (b *Broadcaster[T]) Subscribe(key string, push func(T)) (cancel func()) {
	b.mu.Lock()
	id := b.next
	b.next++
	if b.subs[key] == nil {
		b.subs[key] = make(map[int64]func(T))
	}
	b.subs[key][id] = push
	b.mu.Unlock()

	return func() {
		b.mu.Lock()
		delete(b.subs[key], id)
		if len(b.subs[key]) == 0 {
			delete(b.subs, key)
		}
		b.mu.Unlock()
	}
}

func (b *Broadcaster[T]) Publish(key string, item T) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, push := range b.subs[key] {
		push(item)
	}
}
