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

func (p *pooledConnection) channelWithRoom() *pooledChannel {
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
