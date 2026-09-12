package ingestion

import (
	"sync"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// pendingFetch es la promesa compartida de "alguien ya esta pidiendo esta
// clave ahora mismo" -- ver batchCoalescer.claim. hasData queda en false si
// la clave no tenia velas (simbolo sin dato, mismo significado que estar
// ausente del mapa de resultado).
type pendingFetch struct {
	done    chan struct{}
	candles []domain.Candle
	hasData bool
}

// batchCoalescer evita que 2 llamadas concurrentes a GetCandlesBatch --
// tipicamente 2 escaneres corriendo su ciclo casi al mismo tiempo, con alta
// superposicion de universo entre si -- paguen la misma consulta a Postgres
// para el mismo simbolo. A diferencia del cache de 60s (que solo ayuda entre
// llamadas separadas en el tiempo), esto cubre el caso de 2 pedidos que
// llegan literalmente a la vez y ven el cache vacio los dos. Confirmado en
// vivo el 2026-09-12 como una fuente real de trabajo duplicado.
//
// A proposito NO usa una ventana de tiempo fija (ej. "esperar 1-2s por si
// aparece otro pedido igual") -- eso le agregaria latencia a CUALQUIER
// pedido, incluso cuando no hay ningun solapamiento. El margen real de
// coalescing es la duracion de la consulta en vuelo: se ajusta solo, sin
// inventar un numero.
type batchCoalescer struct {
	mu       sync.Mutex
	inFlight map[string]*pendingFetch
}

func newBatchCoalescer() *batchCoalescer {
	return &batchCoalescer{inFlight: make(map[string]*pendingFetch)}
}

// claim reparte `keys` en dos grupos: los que este caller debe buscar de
// verdad (own, quedan registrados en inFlight hasta que este caller llame
// resolve) y los que ya estan en vuelo por OTRO caller (join, con su
// pendingFetch listo para esperar). Debe llamarse ANTES de disparar
// cualquier consulta real, bajo el mismo lock para que 2 callers nunca
// reclamen la misma clave.
func (c *batchCoalescer) claim(keys []string) (own []string, join map[string]*pendingFetch) {
	c.mu.Lock()
	defer c.mu.Unlock()
	join = make(map[string]*pendingFetch)
	for _, key := range keys {
		if pf, exists := c.inFlight[key]; exists {
			join[key] = pf
			continue
		}
		c.inFlight[key] = &pendingFetch{done: make(chan struct{})}
		own = append(own, key)
	}
	return own, join
}

// resolve entrega el resultado a quien este esperando cada key de `own` y
// libera el registro -- el caller que hizo claim() SIEMPRE debe llamar esto
// (con defer si hace falta), incluso si la consulta real fallo, o los que
// esperan en join() quedarian colgados para siempre.
func (c *batchCoalescer) resolve(own []string, symbolByKey map[string]string, resolved map[string][]domain.Candle) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, key := range own {
		pf := c.inFlight[key]
		if candles, ok := resolved[symbolByKey[key]]; ok {
			pf.candles = candles
			pf.hasData = true
		}
		close(pf.done)
		delete(c.inFlight, key)
	}
}
