package domain

import "sort"

const (
	VolumeProfileSlots    = 960
	VolumeProfileSessions = 20
)

type VolumeProfile struct {
	Symbol     string
	Sessions   int
	Cumulative []float32
}

type SessionVolumes struct {
	bySession map[int32]*[VolumeProfileSlots]int64
}

func NewSessionVolumes() *SessionVolumes {
	return &SessionVolumes{bySession: make(map[int32]*[VolumeProfileSlots]int64)}
}

func (s *SessionVolumes) Add(session int32, slot int, volume int64) {
	if slot < 0 || slot >= VolumeProfileSlots {
		return
	}
	day, ok := s.bySession[session]
	if !ok {
		day = new([VolumeProfileSlots]int64)
		s.bySession[session] = day
	}
	day[slot] += volume
}

func (s *SessionVolumes) Profile(maxSessions int) (int, []float32) {
	sessions := make([]int32, 0, len(s.bySession))
	for session := range s.bySession {
		sessions = append(sessions, session)
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i] > sessions[j] })
	if len(sessions) > maxSessions {
		sessions = sessions[:maxSessions]
	}
	if len(sessions) == 0 {
		return 0, nil
	}
	sum := make([]float64, VolumeProfileSlots)
	for _, session := range sessions {
		var running int64
		for slot, volume := range s.bySession[session] {
			running += volume
			sum[slot] += float64(running)
		}
	}
	cumulative := make([]float32, VolumeProfileSlots)
	for slot := range sum {
		cumulative[slot] = float32(sum[slot] / float64(len(sessions)))
	}
	return len(sessions), cumulative
}
