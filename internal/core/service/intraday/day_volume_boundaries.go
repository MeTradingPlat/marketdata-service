package intraday

import "time"

type DayBoundary int

const (
	PreMarketEnd DayBoundary = iota
	RegularEnd
)

func (t *DayVolumeTracker) SetBoundary(day time.Time, kind DayBoundary, volumes map[string]int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.resetForDayLocked(day)
	target := t.boundaryMapLocked(kind)
	for symbol, volume := range volumes {
		target[symbol] = volume
	}
}

func (t *DayVolumeTracker) Boundary(symbol string, kind DayBoundary) (int64, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	v, ok := t.boundaryMapLocked(kind)[symbol]
	return v, ok
}

func (t *DayVolumeTracker) boundaryMapLocked(kind DayBoundary) map[string]int64 {
	if kind == PreMarketEnd {
		return t.preMarketEnd
	}
	return t.regularEnd
}

func (t *DayVolumeTracker) resetForDayLocked(day time.Time) {
	if t.day.Equal(day) {
		return
	}
	t.day = day
	t.volumes = make(map[string]int64)
	t.preMarketEnd = make(map[string]int64)
	t.regularEnd = make(map[string]int64)
}
