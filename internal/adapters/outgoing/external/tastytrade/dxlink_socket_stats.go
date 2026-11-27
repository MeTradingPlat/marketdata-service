package tastytrade

import (
	"sync"
	"sync/atomic"
	"time"
)

type dxLinkSocketStats struct {
	opensRequested  atomic.Int64
	opensSucceeded  atomic.Int64
	opensTimedOut   atomic.Int64
	opensCanceled   atomic.Int64
	cancelsSent     atomic.Int64
	lateResponses   atomic.Int64
	channelsClosed  atomic.Int64
	lastErrorAtNano atomic.Int64

	mu                sync.Mutex
	serverErrors      map[string]int64
	unknownTypes      map[string]int64
	abandonedChannels map[int]time.Time
	lastCause         string
}

func newDxLinkSocketStats() *dxLinkSocketStats {
	return &dxLinkSocketStats{
		serverErrors:      make(map[string]int64),
		unknownTypes:      make(map[string]int64),
		abandonedChannels: make(map[int]time.Time),
	}
}

func (s *dxLinkSocketStats) reset() {
	s.opensRequested.Store(0)
	s.opensSucceeded.Store(0)
	s.opensTimedOut.Store(0)
	s.opensCanceled.Store(0)
	s.cancelsSent.Store(0)
	s.lateResponses.Store(0)
	s.channelsClosed.Store(0)
	s.lastErrorAtNano.Store(0)
	s.mu.Lock()
	s.serverErrors = make(map[string]int64)
	s.unknownTypes = make(map[string]int64)
	s.abandonedChannels = make(map[int]time.Time)
	s.lastCause = ""
	s.mu.Unlock()
}
