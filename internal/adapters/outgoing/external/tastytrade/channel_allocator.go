package tastytrade

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const defaultMaxConnections = 40

// connectionStaggerDelay espacia la apertura de conexiones nuevas -- sin
// esto, un onboarding grande (ej. el rollout de M1 con el pool recien
// reseteado) pide varias conexiones nuevas en sucesion casi inmediata, y
// TastyTrade rechaza la autenticacion si llegan demasiadas muy rapido
// (confirmado en vivo: "exceeded the configured limit" durante un rollout
// rapido). Mismo remedio que ya usaba CandleChannelAllocator.java
// (CONNECTION_STAGGER_MS), cuyo comentario dice que sin esto ya habia
// disparado un rechazo de autenticacion en vivo dos veces.
const connectionStaggerDelay = 200 * time.Millisecond

// channelAllocator decide donde poner un simbolo nuevo: primero un canal
// con espacio, luego un canal nuevo en una conexion con espacio, y solo si
// ninguna tiene espacio, una conexion nueva -- hasta maxConnections.
type channelAllocator struct {
	connFactory    func(ctx context.Context) (*DxLinkConn, error)
	onNewChannel   func(*dxLinkChannel)
	onReconnect    func(ctx context.Context, pc *pooledConnection)
	maxConnections int

	mu          sync.Mutex // solo contabilidad -- nunca sostenido durante I/O de red
	connections []*pooledConnection

	// growMu serializa UNICAMENTE el camino lento (abrir conexion/canal
	// nuevo -- I/O de red real: handshake TCP/TLS + SETUP/AUTH de dxLink).
	// Mismo motivo por el que CandleChannelAllocator.java usa ReentrantLock
	// en vez de synchronized: sostener un lock compartido mientras se
	// espera una red real bloquea a cualquier otro llamador que solo
	// necesitaba un canal YA disponible en otra conexion, sin ninguna razon
	// real. Go no tiene el pinning de hilos virtuales que tenia ese caso
	// (su scheduler puede mover otras goroutines a otro hilo del SO
	// mientras esta espera), pero el mismo error de diseño -- I/O bajo un
	// lock compartido -- igual serializa el onboarding de miles de simbolos
	// detras de un solo handshake lento si no se separa del lock rapido.
	growMu sync.Mutex

	// lastConnectionOpenedAt: protegido por growMu, no por su propio candado
	// -- solo se lee/escribe dentro del tramo que ya tiene growMu tomado.
	lastConnectionOpenedAt time.Time
}

func newChannelAllocator(connFactory func(ctx context.Context) (*DxLinkConn, error), onNewChannel func(*dxLinkChannel), onReconnect func(ctx context.Context, pc *pooledConnection), maxConnections int) *channelAllocator {
	if maxConnections <= 0 {
		maxConnections = defaultMaxConnections
	}
	return &channelAllocator{
		connFactory: connFactory, onNewChannel: onNewChannel, onReconnect: onReconnect,
		maxConnections: maxConnections,
	}
}

func (a *channelAllocator) allocate(ctx context.Context) (*pooledChannel, error) {
	if ch := a.findRoom(); ch != nil {
		return ch, nil
	}

	a.growMu.Lock()
	defer a.growMu.Unlock()

	if ch := a.findRoom(); ch != nil {
		return ch, nil
	}
	if pc := a.connectionWithRoomForChannel(); pc != nil {
		return a.openChannelOn(ctx, pc)
	}
	if a.connectionCount() >= a.maxConnections {
		return nil, fmt.Errorf("candle pool at connection ceiling (%d)", a.maxConnections)
	}

	a.staggerBeforeNewConnection(ctx)

	conn, err := a.connFactory(ctx)
	if err != nil {
		return nil, fmt.Errorf("opening new pool connection: %w", err)
	}
	pc := newPooledConnection(conn)
	if a.onReconnect != nil {
		conn.OnReconnect(func(rctx context.Context, _ *DxLinkConn) { a.onReconnect(rctx, pc) })
	}
	a.mu.Lock()
	a.connections = append(a.connections, pc)
	a.mu.Unlock()

	return a.openChannelOn(ctx, pc)
}

// staggerBeforeNewConnection espera lo que falte para completar
// connectionStaggerDelay desde que se abrio la ultima conexion nueva --
// llamada ya con growMu tomado, asi que solo un caller a la vez puede
// estar esperando/abriendo, nunca dos en paralelo.
func (a *channelAllocator) staggerBeforeNewConnection(ctx context.Context) {
	if !a.lastConnectionOpenedAt.IsZero() {
		if wait := connectionStaggerDelay - time.Since(a.lastConnectionOpenedAt); wait > 0 {
			select {
			case <-time.After(wait):
			case <-ctx.Done():
			}
		}
	}
	a.lastConnectionOpenedAt = time.Now()
}

func (a *channelAllocator) openChannelOn(ctx context.Context, pc *pooledConnection) (*pooledChannel, error) {
	ch, err := pc.conn.OpenChannel(ctx)
	if err != nil {
		return nil, fmt.Errorf("opening dxlink channel: %w", err)
	}
	if a.onNewChannel != nil {
		a.onNewChannel(ch)
	}
	pooled := newPooledChannel(ch)
	pc.addChannel(pooled)
	return pooled, nil
}

func (a *channelAllocator) findRoom() *pooledChannel {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, pc := range a.connections {
		if ch := pc.channelWithRoom(); ch != nil {
			return ch
		}
	}
	return nil
}

func (a *channelAllocator) connectionWithRoomForChannel() *pooledConnection {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, pc := range a.connections {
		if pc.hasRoomForNewChannel() {
			return pc
		}
	}
	return nil
}

// drainAll saca todas las conexiones del pool y lo deja vacio -- la
// proxima allocate() arranca desde cero, abriendo conexiones nuevas segun
// haga falta en vez de reusar las que trae el llamador.
func (a *channelAllocator) drainAll() []*pooledConnection {
	a.mu.Lock()
	defer a.mu.Unlock()
	conns := a.connections
	a.connections = nil
	return conns
}

// forceReconnectAll fuerza el cierre+reconexion de TODAS las conexiones del
// pool -- ver CandlePool.ForceReconnectAll. Copia la lista bajo lock y
// suelta ANTES de llamar a ForceReconnect (que hace I/O de socket) por el
// mismo motivo que el resto de este archivo evita I/O bajo un.mu compartido.
func (a *channelAllocator) forceReconnectAll(ctx context.Context) {
	a.mu.Lock()
	conns := append([]*pooledConnection(nil), a.connections...)
	a.mu.Unlock()
	for _, pc := range conns {
		pc.conn.ForceReconnect(ctx)
	}
}

func (a *channelAllocator) connectionCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.connections)
}

// allChannels devuelve todos los canales pooled de todas las conexiones --
// copia las listas bajo lock y las suelta antes de que el llamador itere,
// mismo motivo que forceReconnectAll. Usado por CandlePool.
// RefreshLiveSubscriptions para reenviar el Add de cada canal por su cuenta,
// sin abrir ningun canal o conexion nueva.
func (a *channelAllocator) allChannels() []*pooledChannel {
	a.mu.Lock()
	conns := append([]*pooledConnection(nil), a.connections...)
	a.mu.Unlock()

	var channels []*pooledChannel
	for _, pc := range conns {
		pc.mu.Lock()
		channels = append(channels, pc.channels...)
		pc.mu.Unlock()
	}
	return channels
}

func (a *channelAllocator) stats() (connected, total int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, pc := range a.connections {
		total++
		if pc.conn.Connected() {
			connected++
		}
	}
	return connected, total
}
