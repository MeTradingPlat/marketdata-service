package tastytrade

import "time"

const maxAbandonedChannels = 256

func (s *dxLinkSocketStats) recordAbandonedChannel(channel int, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.abandonedChannels == nil {
		s.abandonedChannels = make(map[int]time.Time)
	}
	if len(s.abandonedChannels) >= maxAbandonedChannels {
		evictOldestAbandoned(s.abandonedChannels)
	}
	s.abandonedChannels[channel] = now
}

func evictOldestAbandoned(m map[int]time.Time) {
	oldestID := 0
	var oldestAt time.Time
	for id, at := range m {
		if oldestAt.IsZero() || at.Before(oldestAt) {
			oldestID, oldestAt = id, at
		}
	}
	delete(m, oldestID)
}

func (s *dxLinkSocketStats) abandonedAge(channel int, now time.Time) (time.Duration, bool) {
	s.mu.Lock()
	at, ok := s.abandonedChannels[channel]
	s.mu.Unlock()
	if !ok {
		return 0, false
	}
	return now.Sub(at), true
}
