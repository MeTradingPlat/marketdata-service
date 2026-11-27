package tastytrade

import "time"

func (s *dxLinkSocketStats) recordServerError(errType string, now time.Time) {
	s.lastErrorAtNano.Store(now.UnixNano())
	s.mu.Lock()
	if s.serverErrors == nil {
		s.serverErrors = make(map[string]int64)
	}
	s.serverErrors[errType]++
	s.mu.Unlock()
}

func (s *dxLinkSocketStats) recordUnknownType(msgType string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unknownTypes == nil {
		s.unknownTypes = make(map[string]int64)
	}
	s.unknownTypes[msgType]++
	return s.unknownTypes[msgType] == 1
}

func (s *dxLinkSocketStats) setLastTimeoutCause(cause string) {
	s.mu.Lock()
	s.lastCause = cause
	s.mu.Unlock()
}

func (s *dxLinkSocketStats) lastTimeoutCause() string {
	return lockedString(&s.mu, &s.lastCause)
}

func (s *dxLinkSocketStats) lastErrorAt() time.Time {
	return unixTimeFromNano(s.lastErrorAtNano.Load())
}
