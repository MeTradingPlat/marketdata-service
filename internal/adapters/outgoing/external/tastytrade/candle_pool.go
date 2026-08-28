package tastytrade

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/rs/zerolog/log"
)

const (
	historyDefaultWait = 15 * time.Second
	// historyDeepWait es para el probeo de profundidad MAXIMA (sin watermark
	// todavia, p. ej. FetchHistoryDeep): una rafaga de miles de velas D1/H1
	// puede tardar mas que los 15s del fetch incremental de 10 dias --
	// confirmado en vivo: la espera corta cortaba la historia de simbolos
	// con mucha profundidad y dejaba el backfill truncado para siempre (el
	// incremental solo trae barras NUEVAS, nunca re-probea hacia atras).
	historyDeepWait = 90 * time.Second

	// historyBatchDeepGapThreshold: si algun simbolo del lote necesita
	// ponerse al dia con mas de esto, el lote entero espera historyDeepWait
	// en vez de historyDefaultWait -- confirmado en vivo el 2026-08-19:
	// FetchHistoryBatch (el barrido de RunSweepPhase) nunca tuvo el
	// equivalente de este ajuste que ya existe para el fetch individual (ver
	// historyDeepWait arriba). Con watermarks atrasados varias horas (ej.
	// tras un simbolo silenciosamente estancado unos dias), el lote se
	// asentaba a los 15s con lo que hubiera llegado hasta ahi, guardaba esa
	// porcion parcial como verified=true y avanzaba el watermark hasta ese
	// punto -- el resto del hueco (desde la apertura hasta donde se corto)
	// quedaba para siempre con los datos provisionales del stream en vivo,
	// porque el proximo fetch incremental nunca vuelve a mirar atras del
	// watermark ya avanzado (confirmado en real: SW/SFM/DNTH/GKOS con
	// decenas de velas M1 de la apertura sin verificar horas despues de 3
	// reinicios). 1h es varias veces el hueco de un reinicio normal
	// (minutos) pero bien por debajo de una sesion completa.
	historyBatchDeepGapThreshold = time.Hour

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
func (p *CandlePool) SubscribeLive(ctx context.Context, symbol string, from time.Time, onClosed func(domain.Candle), onTick func(domain.Candle)) error {
	ch, err := p.allocator.allocate(ctx)
	if err != nil {
		return fmt.Errorf("allocating channel for %s live M1: %w", symbol, err)
	}

	p.liveMu.Lock()
	p.liveSubs[symbol] = onClosed
	p.liveTicks[symbol] = onTick
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
// formacion; un timestamp MAS NUEVO significa que la anterior ya cerro. En
// los dos casos se reenvia la vela en formacion actualizada (dispatchTick)
// -- los graficos la necesitan tick a tick, no solo al cierre.
//
// Un timestamp MAS VIEJO que la vela en formacion es un caso aparte: una
// correccion tardia de una vela YA cerrada (el trade real ocurrio en ese
// minuto pero el reporte del exchange/SIP llego despues de que ya cerramos
// el minuto siguiente -- documentado, no un caso raro: "el primer trade del
// minuto nuevo puede llegar varios segundos despues del cierre formal si el
// simbolo opera poco o hay demoras tecnicas"). Antes, el simple "timestamp
// distinto" de mas abajo confundia esto con una vela nueva -- cerraba de
// golpe la vela en formacion REAL con datos a medio completar y reabria la
// vieja como si fuera la actual. Confirmado en vivo el 2026-08-27/28: el
// volumen real de un pico de un minuto tardaba varios ciclos del escaner en
// reflejarse completo rio abajo. Ahora se despacha aparte via dispatchClosed,
// SIN tocar current -- fusionada contra lastClosed[symbol] (la ultima vela
// que SI cerro de verdad) para no perder los campos que dxLink no reenvio
// (formato COMPACT: solo manda lo que cambio). Si lastClosed no tiene ese
// timestamp exacto (la correccion apunta mas atras de una vela, caso raro),
// se fusiona sobre una base vacia como antes -- mejor esfuerzo, no hay de
// donde mas sacar el resto de los campos.
func (p *CandlePool) handleLiveEvent(symbol string, ev rawCandleEvent) {
	p.currentMu.Lock()
	prev, exists := p.current[symbol]
	if exists && ev.Timestamp.Before(prev.Timestamp) {
		base := domain.Candle{}
		if last, ok := p.lastClosed[symbol]; ok && last.Timestamp.Equal(ev.Timestamp) {
			base = last
		}
		corrected := mergeCandle(base, ev, symbol, domain.M1)
		p.lastClosed[symbol] = corrected
		p.currentMu.Unlock()
		p.dispatchClosed(symbol, corrected)
		return
	}

	var forming domain.Candle
	if exists && !prev.Timestamp.Equal(ev.Timestamp) {
		// Un minuto sin ticks no deja vela -- por diseño: la vela solo se
		// cierra cuando llega un tick del minuto siguiente, y un minuto
		// muerto (sin operaciones) no genera ningun evento. Sintetizar velas
		// planas para los minutos intermedios (open=close al ultimo precio,
		// volumen 0) parecia dar continuidad al grafico, pero el usuario lo
		// rechazo en vivo el 2026-08-19: barras identicas donde no cambio
		// nada ensucian la serie -- el chart debe mostrar barras solo donde
		// hubo movimiento real (como antes de a19301a).
		closed := prev
		p.current[symbol] = mergeCandle(domain.Candle{}, ev, symbol, domain.M1)
		p.lastClosed[symbol] = closed
		forming = p.current[symbol]
		p.currentMu.Unlock()
		p.dispatchClosed(symbol, closed)
		p.dispatchTick(symbol, forming)
		return
	}
	forming = mergeCandle(prev, ev, symbol, domain.M1)
	p.current[symbol] = forming
	p.currentMu.Unlock()
	p.dispatchTick(symbol, forming)
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

// dispatchTick reenvia la vela en formacion tras cada tick -- el suscriptor
// (StreamLive -> Broadcaster) descarta si su canal va lleno, asi un cliente
// lento jamas frena el merge de ticks de la conexion DxLink. El filtro es
// SOLO "tiene cierre": un tick parcial (minuto sin trades, primer evento
// del periodo con OHLC incompleto) se dibuja plano en el grafico y el
// siguiente tick lo completa -- descartarlo por IsComplete dejaba huecos en
// la serie en vivo (minutos enteros saltados).
func (p *CandlePool) dispatchTick(symbol string, c domain.Candle) {
	if c.Close == 0 {
		return
	}
	p.liveMu.Lock()
	cb := p.liveTicks[symbol]
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
		tick := p.liveTicks[symbol]
		p.liveMu.Unlock()
		if cb == nil {
			continue
		}
		if err := p.SubscribeLive(ctx, symbol, resumeFrom[symbol], cb, tick); err != nil {
			// El simbolo quedo MUERTO de verdad (sin canal que lo sirva) --
			// sacarlo del estado live del pool para que LiveSubscribed() lo
			// reporte como caido y el reconciliador (cmd/api) lo resuscriba
			// (confirmado en vivo el 2026-08-18: OSRH quedo mudo en silencio
			// tras un resubscribe fallido y nada lo reintentaba porque la
			// entrada seguia en el mapa).
			p.liveMu.Lock()
			delete(p.liveSubs, symbol)
			delete(p.liveTicks, symbol)
			p.liveMu.Unlock()
			log.Error().Err(err).Str("symbol", symbol).Msg("failed to resubscribe live candle after reconnect, dropped from live state")
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
	// lastClosed tambien se limpia aca -- el hueco hasta el proximo
	// SubscribeLive es de horas (mercado cerrado de por medio), asi que
	// cualquier tick tardio que llegue del otro lado ya pertenece a un dia
	// distinto y no debe fusionarse contra la vela vieja.
	p.lastClosed = make(map[string]domain.Candle)
	p.currentMu.Unlock()

	log.Info().Int("symbols", len(stopped)).Msg("stopped all live M1 subscriptions for the maintenance window")
}

// CloseAllConnections desuscribe todo lo que haya ocupado en cada canal y
// cierra las conexiones fisicas del pool sin esperar -- se usa en las
// fronteras D1->H1->M1 del barrido nocturno para que cada fase arranque
// con cero sesiones abiertas ante TastyTrade, en vez de arrastrar las
// conexiones que uso la fase anterior justo cuando la siguiente intenta
// abrir varias de golpe (confirmado en vivo: asi se supero el limite de
// sesiones de TastyTrade durante un rollout de M1 rapido). DxLinkConn.Close
// marca cada conexion como cierre intencional para que no dispare su
// propia reconexion automatica.
func (p *CandlePool) CloseAllConnections() {
	conns := p.allocator.drainAll()

	p.dispatchMu.Lock()
	p.dispatch = make(map[string]dispatchEntry)
	p.dispatchMu.Unlock()

	p.liveMu.Lock()
	p.liveSubs = make(map[string]func(domain.Candle))
	p.liveMu.Unlock()

	p.currentMu.Lock()
	p.current = make(map[string]domain.Candle)
	p.currentMu.Unlock()

	for _, pc := range conns {
		pc.mu.Lock()
		channels := append([]*pooledChannel(nil), pc.channels...)
		pc.mu.Unlock()

		for _, ch := range channels {
			ch.mu.Lock()
			keys := make([]string, 0, len(ch.symbols))
			for key := range ch.symbols {
				keys = append(keys, key)
			}
			ch.mu.Unlock()
			for _, key := range keys {
				if symbol, tf, ok := parseCandleKey(key); ok {
					_ = ch.channel.unsubscribe(symbol, tf)
				}
			}
		}
		pc.conn.Close()
	}

	log.Info().Int("connections", len(conns)).Msg("closed all dxlink connections at phase boundary")
}

// hasLiveSub dice si un simbolo ya tiene una suscripcion M1 en vivo activa.
func (p *CandlePool) hasLiveSub(symbol string) bool {
	p.liveMu.Lock()
	defer p.liveMu.Unlock()
	_, ok := p.liveSubs[symbol]
	return ok
}

// LiveSubscribed dice si el stream M1 del simbolo esta ACTUALMENTE
// registrado en el pool -- el reconciliador (cmd/api) lo usa para
// detectar las muertes silenciosas (un resubscribe fallido tras una
// reconexion deja el stream mudo sin error visible, confirmado en vivo el
// 2026-08-18 con OSRH) y resuscribirlas aunque la marca del ingestor siga
// en true.
func (p *CandlePool) LiveSubscribed(symbol string) bool {
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

// batchHistoryWait elige historyDeepWait para el lote entero si ALGUN
// simbolo necesita ponerse al dia con mas de historyBatchDeepGapThreshold --
// ver el comentario de esa constante arriba.
func batchHistoryWait(froms map[string]time.Time) time.Duration {
	now := time.Now()
	for _, from := range froms {
		if now.Sub(from) > historyBatchDeepGapThreshold {
			return historyDeepWait
		}
	}
	return historyDefaultWait
}

// FetchHistoryBatch pide el historial de un LOTE de simbolos en una sola
// suscripcion DxLink (cada uno con su propio FromTime) -- es el
// agrupamiento original del pool de Java (100 simbolos por canal) que el
// barrido nocturno usa para no pagar 13k round-trips de add/remove por
// timeframe. Mismo ciclo que FetchHistory: add del lote, rafaga, remove
// del lote, dispatch por simbolo para enrutar cada evento al collector.
func (p *CandlePool) FetchHistoryBatch(ctx context.Context, tf domain.Timeframe, froms map[string]time.Time) (map[string][]domain.Candle, error) {
	if len(froms) == 0 {
		return map[string][]domain.Candle{}, nil
	}
	symbols := make([]string, 0, len(froms))
	for symbol := range froms {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)

	// Mismo lock por clave que FetchHistory, en orden lexicografico para
	// no interbloquearse con un FetchHistory por-simbolo concurrente.
	unlockAll := func() {
		for _, symbol := range symbols {
			if lockVal, ok := p.historyLocks.Load(candleKey(symbol, tf)); ok {
				lockVal.(*sync.Mutex).Unlock()
			}
		}
	}
	for _, symbol := range symbols {
		lockVal, _ := p.historyLocks.LoadOrStore(candleKey(symbol, tf), &sync.Mutex{})
		lockVal.(*sync.Mutex).Lock()
	}

	ch, err := p.allocator.allocate(ctx)
	if err != nil {
		unlockAll()
		return nil, fmt.Errorf("allocating channel for %s history batch: %w", tf, err)
	}

	for _, symbol := range symbols {
		ch.occupy(candleKey(symbol, tf))
	}

	collector := newBatchHistoryCollector(tf)
	dispatchIDs := make(map[string]uint64, len(symbols))
	for _, symbol := range symbols {
		sym := symbol
		dispatchIDs[symbol] = p.registerDispatch(symbol, tf, "history-batch", func(ev rawCandleEvent) { collector.onCandle(sym, ev) })
	}

	cleanup := func() {
		for _, symbol := range symbols {
			ch.release(candleKey(symbol, tf))
		}
		_ = ch.channel.unsubscribeHistoryBatch(symbols, tf)
		time.AfterFunc(unsubscribeDrainPeriod, func() {
			for _, symbol := range symbols {
				p.unregisterDispatchIfCurrent(symbol, tf, dispatchIDs[symbol], "history-batch")
			}
		})
	}

	if err := ch.channel.subscribeHistoryBatch(symbols, tf, froms); err != nil {
		cleanup()
		unlockAll()
		return nil, fmt.Errorf("subscribing history batch: %w", err)
	}
	if err := waitForData(ctx, collector.settled, batchHistoryWait(froms)); err != nil {
		cleanup()
		unlockAll()
		return nil, err
	}
	result := collector.complete()
	cleanup()
	unlockAll()
	return result, nil
}

// FetchHistoryDeep es el probe de profundidad maxima -- misma logica que
// FetchHistory pero con historyDeepWait: sin un watermark que acote, la
// rafaga historica completa de un simbolo profundo puede superar los 15s
// del fetch incremental (ver historyDeepWait).
func (p *CandlePool) FetchHistoryDeep(ctx context.Context, symbol string, tf domain.Timeframe, from time.Time) ([]domain.Candle, error) {
	return p.fetchHistory(ctx, symbol, tf, from, historyDeepWait)
}

func (p *CandlePool) FetchHistory(ctx context.Context, symbol string, tf domain.Timeframe, from time.Time) ([]domain.Candle, error) {
	return p.fetchHistory(ctx, symbol, tf, from, historyDefaultWait)
}

func (p *CandlePool) fetchHistory(ctx context.Context, symbol string, tf domain.Timeframe, from time.Time, wait time.Duration) ([]domain.Candle, error) {
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
	if err := waitForData(ctx, collector.settled, wait); err != nil {
		cleanup()
		return nil, err
	}
	result := collector.complete()
	cleanup()
	return result, nil
}
