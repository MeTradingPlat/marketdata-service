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

const (
	historyDefaultWait = 15 * time.Second

	// unsubscribeDrainPeriod: dxLink no confirma cuando un FEED_SUBSCRIPTION
	// remove ya surtio efecto del lado del servidor (verificado contra la
	// especificacion oficial: no hay ACK, y el spec no dice nada sobre
	// eventos que ya estaban en camino). Mantener el dispatch registrado un
	// rato mas despues de mandar el remove absorbe esos rezagados en vez de
	// contarlos como huerfanos -- no cambia el resultado ya devuelto, solo
	// reduce el ruido de una condicion de carrera inherente al protocolo.
	unsubscribeDrainPeriod = 250 * time.Millisecond
)

type dispatchEntry struct {
	id      uint64
	handler func(rawCandleEvent)
}

// CandlePool reparte simbolos entre canales/conexiones DxLink pooled (via
// channelAllocator) y enruta cada evento crudo que llega a la suscripcion
// (en vivo o historial) que le corresponde -- un canal pooled sirve varias
// suscripciones distintas a la vez, asi que el dispatch es un registro
// central por simbolo+temporalidad, no un callback fijo por canal.
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

	currentMu sync.Mutex
	current   map[string]domain.Candle

	// orphanEvents cuenta eventos que llegan para un simbolo+temporalidad
	// que ya no tiene handler registrado -- si esto crece SIN PARAR con el
	// tiempo, confirma una fuga de suscripcion server-side (lo que se vio y
	// se corrigio antes con el formato normalizado del simbolo). Una rafaga
	// acotada que no sigue creciendo es la condicion de carrera esperada
	// del unsubscribe (ver unsubscribeDrainPeriod), no una fuga.
	orphanEvents int64
}

func NewCandlePool(connFactory func(ctx context.Context) (*DxLinkConn, error), maxConnections int) *CandlePool {
	p := &CandlePool{
		dispatch: make(map[string]dispatchEntry),
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

// SubscribeLive es la unica suscripcion M1 de un simbolo: se abre con
// FromTime = from (para retomar exactamente donde quedo el ultimo dato
// guardado) y se queda abierta para siempre, sin un FetchHistory M1
// separado antes ni un remove/add de por medio -- ver el comentario de
// subscribeLive sobre por que esa secuencia dejaba el streaming mudo.
func (p *CandlePool) SubscribeLive(ctx context.Context, symbol string, from time.Time, onClosed func(domain.Candle)) error {
	ch, err := p.allocator.allocate(ctx)
	if err != nil {
		return fmt.Errorf("allocating channel for %s live M1: %w", symbol, err)
	}

	p.liveMu.Lock()
	p.liveSubs[symbol] = onClosed
	p.liveMu.Unlock()

	_ = p.registerDispatch(symbol, domain.M1, "live", func(ev rawCandleEvent) { p.handleLiveEvent(symbol, ev) })
	ch.occupy(candleKey(symbol, domain.M1))

	if err := ch.channel.subscribeLive(symbol, domain.M1, from); err != nil {
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

	// La vela a medio formar de cada simbolo queda incompleta -- le faltan
	// los ticks de los segundos que la conexion estuvo caida -- asi que se
	// descarta de current, pero su timestamp se guarda como punto de
	// retomada: resuscribir con FromTime = ese timestamp le pide a dxLink
	// que repita desde ahi (la vela incompleta incluida, esta vez completa)
	// en vez de dejar el hueco perdido para siempre.
	p.currentMu.Lock()
	resumeFrom := make(map[string]time.Time, len(symbols))
	for _, symbol := range symbols {
		if prev, ok := p.current[symbol]; ok {
			resumeFrom[symbol] = prev.Timestamp
		}
		delete(p.current, symbol)
	}
	p.currentMu.Unlock()

	for _, symbol := range symbols {
		p.liveMu.Lock()
		cb := p.liveSubs[symbol]
		p.liveMu.Unlock()
		if cb == nil {
			continue
		}
		if err := p.SubscribeLive(ctx, symbol, resumeFrom[symbol], cb); err != nil {
			log.Error().Err(err).Str("symbol", symbol).Msg("failed to resubscribe live candle after reconnect")
		}
	}
}

// StopAllLive desuscribe TODAS las suscripciones M1 en vivo del pool -- se
// usa antes del barrido pesado de D1/H1 sobre el universo completo (en una
// hora sin movimiento de mercado, ver runOnce), para que ese barrido no
// compita por conexiones/canales con miles de suscripciones en vivo. Es
// seguro (no repite la colision que causaba el congelamiento) porque el
// hueco entre este remove y el proximo SubscribeLive va a ser de horas,
// no milisegundos -- dxFeed ya proceso el remove de sobra para cuando
// llegue el add. Cada simbolo retoma solo desde su propio watermark al
// resuscribirse, sin perder nada (el mercado estuvo cerrado mientras tanto).
func (p *CandlePool) StopAllLive(ctx context.Context) {
	p.allocator.mu.Lock()
	conns := append([]*pooledConnection(nil), p.allocator.connections...)
	p.allocator.mu.Unlock()

	var stopped []string
	for _, pc := range conns {
		pc.mu.Lock()
		channels := append([]*pooledChannel(nil), pc.channels...)
		pc.mu.Unlock()
		for _, ch := range channels {
			for _, symbol := range ch.liveSymbols() {
				_ = ch.channel.unsubscribe(symbol, domain.M1)
				ch.release(candleKey(symbol, domain.M1))
				stopped = append(stopped, symbol)
			}
		}
	}

	p.dispatchMu.Lock()
	for _, symbol := range stopped {
		delete(p.dispatch, candleKey(symbol, domain.M1))
	}
	p.dispatchMu.Unlock()

	p.liveMu.Lock()
	p.liveSubs = make(map[string]func(domain.Candle))
	p.liveMu.Unlock()

	p.currentMu.Lock()
	p.current = make(map[string]domain.Candle)
	p.currentMu.Unlock()

	log.Info().Int("symbols", len(stopped)).Msg("stopped all live M1 subscriptions for the maintenance window")
}

// hasLiveSub dice si un simbolo ya tiene una suscripcion M1 en vivo activa.
func (p *CandlePool) hasLiveSub(symbol string) bool {
	p.liveMu.Lock()
	defer p.liveMu.Unlock()
	_, ok := p.liveSubs[symbol]
	return ok
}

// CurrentCandle devuelve la vela M1 en formacion ahora mismo -- precio y
// volumen mas frescos que cualquier vela ya cerrada en la BD, se actualiza
// en cada tick sin esperar a que cierre el minuto.
func (p *CandlePool) CurrentCandle(symbol string) (domain.Candle, bool) {
	p.currentMu.Lock()
	defer p.currentMu.Unlock()
	c, ok := p.current[symbol]
	return c, ok
}

func (p *CandlePool) FetchHistory(ctx context.Context, symbol string, tf domain.Timeframe, from time.Time) ([]domain.Candle, error) {
	// Un fetch M1 puntual para un simbolo que YA esta en vivo competiria por
	// la MISMA suscripcion server-side (dxFeed fusiona el "add" de esta
	// peticion con el "add" en vivo en un solo tema), y el "remove" del
	// cleanup de este fetch se lleva TAMBIEN la suscripcion en vivo por
	// delante -- confirmado en vivo: el streaming de simbolos que ademas
	// caian dentro de un lote de backfill se quedaba mudo para siempre, sin
	// ningun error, porque el TCP seguia perfectamente sano sirviendo el
	// resto del trafico del lote. El stream en vivo (mas el relleno de
	// huecos tras reconexion) ya es la fuente autoritativa hacia adelante
	// para M1, asi que aqui no hay nada que este fetch pueda aportar.
	if tf == domain.M1 && p.hasLiveSub(symbol) {
		return nil, nil
	}

	key := candleKey(symbol, tf)

	lockVal, _ := p.historyLocks.LoadOrStore(key, &sync.Mutex{})
	lock := lockVal.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	ch, err := p.allocator.allocate(ctx)
	if err != nil {
		return nil, fmt.Errorf("allocating channel for %s %s history: %w", symbol, tf, err)
	}

	ch.occupy(key)

	collector := newHistoryCollector(symbol, tf)
	dispatchID := p.registerDispatch(symbol, tf, "history", collector.onCandle)

	// El remove va primero (le da al servidor la maxima ventaja de tiempo
	// para procesarlo), el dispatch se borra despues de un rato -- ver
	// unsubscribeDrainPeriod. unregisterDispatchIfCurrent solo borra si
	// nadie volvio a registrar esa misma clave mientras tanto.
	cleanup := func() {
		ch.release(key)
		_ = ch.channel.unsubscribe(symbol, tf)
		time.AfterFunc(unsubscribeDrainPeriod, func() {
			p.unregisterDispatchIfCurrent(symbol, tf, dispatchID, "history")
		})
	}

	if err := ch.channel.subscribeHistory(symbol, tf, from); err != nil {
		cleanup()
		return nil, fmt.Errorf("subscribing history: %w", err)
	}
	if err := waitForData(ctx, collector.settled, historyDefaultWait); err != nil {
		cleanup()
		return nil, err
	}
	result := collector.complete()
	cleanup()
	return result, nil
}
