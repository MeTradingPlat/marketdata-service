package tastytrade

import (
	"testing"
)

func TestChannelAllocator_PackingLimits(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	conn := newConnectedDxLinkConn(t, wsURL)
	pc := newPooledConnection(conn)
	for i := 0; i < 11; i++ {
		pc.addChannel(&pooledChannel{})
	}

	a := newChannelAllocator(nil, nil, nil, 10)
	a.connections = append(a.connections, pc)

	a.setMaxChannelsPerConn(12)
	if got := a.connectionWithRoomForChannel(); got != pc {
		t.Fatalf("expected connection with 11 channels to have room when max is 12, got %v", got)
	}

	pc.addChannel(&pooledChannel{})
	a.setMaxChannelsPerConn(8)
	if got := a.connectionWithRoomForChannel(); got != nil {
		t.Fatalf("expected connection with 12 channels to have no room when max is 8, got %v", got)
	}
}
