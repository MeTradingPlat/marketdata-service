package intraday

import (
	"slices"
	"sort"
	"time"
)

type minuteEntry struct {
	minute int64
	volume int64
}

type minuteRegistry map[string][]minuteEntry

func minuteKey(ts time.Time) int64 {
	return ts.Truncate(time.Minute).Unix()
}

func (r minuteRegistry) record(symbol string, minute, volume int64) int64 {
	entries := r[symbol]
	idx := sort.Search(len(entries), func(i int) bool { return entries[i].minute >= minute })
	if idx < len(entries) && entries[idx].minute == minute {
		delta := volume - entries[idx].volume
		entries[idx].volume = volume
		return delta
	}
	r[symbol] = slices.Insert(entries, idx, minuteEntry{minute: minute, volume: volume})
	return volume
}

func (r minuteRegistry) hasNewerInRange(symbol string, afterMin, beforeMin int64) bool {
	entries := r[symbol]
	idx := sort.Search(len(entries), func(i int) bool { return entries[i].minute > afterMin })
	return idx < len(entries) && (beforeMin == 0 || entries[idx].minute < beforeMin)
}

func (r minuteRegistry) isOldestFrom(symbol string, min, fromMin int64) bool {
	entries := r[symbol]
	idx := sort.Search(len(entries), func(i int) bool { return entries[i].minute >= fromMin })
	return idx < len(entries) && entries[idx].minute == min
}
