package tastytrade

import "sync"

const maxChannelsPerConnection = 8

type pooledConnection struct {
	conn *DxLinkConn

	mu       sync.Mutex
	channels []*pooledChannel
}

func newPooledConnection(conn *DxLinkConn) *pooledConnection {
	return &pooledConnection{conn: conn}
}

// channelWithRoom nunca devuelve un canal de una conexion caida/reconectando
// -- ambos chequeos de "hay lugar" (por simbolo y por cantidad de canales,
// ver hasRoomForNewChannel) solo contaban simbolos/canales, nunca si el
// socket de abajo seguia vivo. Con una conexion en medio de reconnectLoop
// (puede tardar hasta 15 min si el rechazo fue por limite de sesiones, ver
// session_breaker.go), allocate() la seguia devolviendo como "con lugar" una
// y otra vez: cada intento de suscribir un simbolo nuevo fallaba contra ESA
// MISMA conexion muerta en vez de abrir una conexion nueva con las otras
// ~39 disponibles, dejando simbolos completos sin poder engancharse hasta
// que esa conexion en particular terminara de reconectar por su cuenta -- o,
// si eso nunca pasaba, hasta un reinicio manual del contenedor.
func (p *pooledConnection) channelWithRoom() *pooledChannel {
	if !p.conn.Connected() {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, ch := range p.channels {
		if ch.hasRoom() {
			return ch
		}
	}
	return nil
}

func (p *pooledConnection) hasRoomForNewChannel() bool {
	if !p.conn.Connected() {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.channels) < maxChannelsPerConnection
}

func (p *pooledConnection) addChannel(ch *pooledChannel) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.channels = append(p.channels, ch)
}

// liveSymbols y reset se usan juntos al reconectar: los canales viejos
// quedan invalidos (IDs de un socket que ya no existe), asi que se leen
// los simbolos en vivo que había que restaurar ANTES de vaciar la lista.
func (p *pooledConnection) liveSymbols() []string {
	p.mu.Lock()
	channels := append([]*pooledChannel(nil), p.channels...)
	p.mu.Unlock()

	var symbols []string
	for _, ch := range channels {
		symbols = append(symbols, ch.liveSymbols()...)
	}
	return symbols
}

func (p *pooledConnection) reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.channels = nil
}
