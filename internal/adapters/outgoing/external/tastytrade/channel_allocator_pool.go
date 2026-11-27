package tastytrade

import (
	"context"
)

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
		pc.conn.noteCloseReason("watchdog_forced_reconnect")
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
		for _, ch := range pc.channels {
			if !ch.isRetired() {
				channels = append(channels, ch)
			}
		}
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

func (a *channelAllocator) owns(ch *pooledChannel) bool {
	a.mu.Lock()
	conns := append([]*pooledConnection(nil), a.connections...)
	a.mu.Unlock()
	for _, pc := range conns {
		if pc.owns(ch) {
			return true
		}
	}
	return false
}

func (a *channelAllocator) setMaxChannelsPerConn(n int) {
	if n <= 0 {
		return
	}
	_ = a.lockGrow(context.Background())
	defer a.unlockGrow()
	a.maxChannelsPerConn = n
}

func (a *channelAllocator) releaseKey(key string) {
	for _, ch := range a.allChannels() {
		ch.release(key)
	}
}

func (a *channelAllocator) removeChannel(ch *pooledChannel) bool {
	if ch == nil {
		return false
	}
	a.mu.Lock()
	conns := append([]*pooledConnection(nil), a.connections...)
	a.mu.Unlock()
	for _, pc := range conns {
		if pc.removeChannel(ch) {
			pc.conn.CloseChannel(ch.channel)
			return true
		}
	}
	return false
}
