package tastytrade

import (
	"context"
	"fmt"
	"sync"
)

const defaultMaxConnections = 40

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

func (a *channelAllocator) connectionCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.connections)
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
