package tastytrade

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// CandlePool reparte simbolos entre canales/conexiones DxLink pooled (via
// channelAllocator) y enruta cada evento crudo que llega a la suscripcion
// (en vivo o historial) que le corresponde -- un canal pooled sirve varias
// suscripciones distintas a la vez, asi que el dispatch es un registro
// central por simbolo+temporalidad, no un callback fijo por canal.
//
// Los metodos de CandlePool estan agrupados por responsabilidad en archivos
// separados del mismo paquete: candle_dispatch.go (registro de dispatch),
// candle_live.go (ciclo de vida de suscripciones en vivo), candle_reconnect.go
// (reconexion), candle_shutdown.go (apagado masivo) y candle_history.go
// (orquestacion de fetch historico).
type CandlePool struct {
	allocator *channelAllocator

	dispatchMu  sync.RWMutex
	dispatch    map[string]dispatchEntry
	dispatchSeq uint64

	// historyLocks serializa FetchHistory por simbolo+temporalidad -- dos
	// llamadas concurrentes para la MISMA clave (confirmado en vivo: el
	// backfill en lote y el catch-up diario corren a la vez al arrancar y
	// se solapan en los mismos simbolos) se pisaban el registro de
	// dispatch, y como dxFeed fusiona varios "add" al mismo tema en una
	// sola suscripcion, el "remove" de la primera mataba tambien la de la
	// segunda sin que su handler llegara a desregistrarse -- la
	// suscripcion quedaba viva en el servidor mandando actualizaciones
	// reales para siempre, sin nadie escuchando (huerfanos que no paran de
	// crecer, a diferencia de la rafaga acotada de unsubscribeDrainPeriod).
	historyLocks sync.Map // map[string]*sync.Mutex

	liveMu   sync.Mutex
	liveSubs map[string]func(domain.Candle)
	// liveTicks reenvia la vela M1 EN FORMACION tras cada tick -- el canal
	// que alimenta la vela en formacion de los graficos (M1 directo, H1/D1
	// por agregacion en candle_ws_session.go). Nil para simbolos sin
	// suscripcion en vivo.
	liveTicks map[string]func(domain.Candle)

	currentMu sync.Mutex
	current   map[string]domain.Candle
	// lastClosed guarda la ULTIMA vela ya cerrada por simbolo (un solo
	// slot, no historia completa) -- base para fusionar una correccion
	// tardia que llega en formato COMPACT (solo trae los campos que
	// cambiaron): sin esta base, un campo que dxLink no reenvio se pierde
	// en vez de conservar el ultimo valor conocido. Protegido por currentMu,
	// no un mutex aparte -- ambos mapas cambian juntos en el mismo evento.
	lastClosed map[string]domain.Candle

	// lastLiveEventAtUnixNano: se toca en CADA evento M1 en vivo que procesa
	// handleLiveEvent, sin importar el simbolo -- LiveDataWatchdog lo usa
	// para medir "hace cuanto no llega NINGUNA vela nueva de NINGUN simbolo",
	// la unica señal que de verdad importa (ver live_data_watchdog.go). A
	// diferencia de lastMessageAtUnixNano de cada DxLinkConn (que se toca con
	// CUALQUIER mensaje, KEEPALIVE incluido), esta se toca solo con datos
	// reales -- confirmado en vivo el 2026-08-31: el socket seguia
	// respondiendo KEEPALIVE con normalidad 3+ horas despues de que la
	// suscripcion de datos se murio en silencio, asi que lastMessageAtUnixNano
	// nunca lo detecto.
	lastLiveEventAtUnixNano atomic.Int64

	// orphanEvents cuenta eventos que llegan para un simbolo+temporalidad
	// que ya no tiene handler registrado -- si esto crece SIN PARAR con el
	// tiempo, confirma una fuga de suscripcion server-side (lo que se vio y
	// se corrigio antes con el formato normalizado del simbolo). Una rafaga
	// acotada que no sigue creciendo es la condicion de carrera esperada
	// del unsubscribe (ver unsubscribeDrainPeriod), no una fuga.
	orphanEvents int64

	// refreshCycle alterna que mitad de canales se refresca en cada llamada
	// a RefreshLiveSubscriptions -- ver el comentario ahi mismo sobre por
	// que partir el barrido en dos grupos que se turnan reduce a la mitad
	// los mensajes de suscripcion por minuto.
	refreshCycle atomic.Uint64
}

func NewCandlePool(connFactory func(ctx context.Context) (*DxLinkConn, error), maxConnections int) *CandlePool {
	p := &CandlePool{
		dispatch:   make(map[string]dispatchEntry),
		liveSubs:   make(map[string]func(domain.Candle)),
		liveTicks:  make(map[string]func(domain.Candle)),
		current:    make(map[string]domain.Candle),
		lastClosed: make(map[string]domain.Candle),
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
