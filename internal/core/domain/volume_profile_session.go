package domain

import "time"

const sessionOpenMinuteET = 4 * 60

func SlotOf(t time.Time, loc *time.Location) (session int32, slot int, ok bool) {
	et := t.In(loc)
	slot = et.Hour()*60 + et.Minute() - sessionOpenMinuteET
	if slot < 0 || slot >= VolumeProfileSlots {
		return 0, 0, false
	}
	return int32(et.Year()*10000 + int(et.Month())*100 + et.Day()), slot, true
}

func StartOfDayET(now time.Time, loc *time.Location) time.Time {
	et := now.In(loc)
	return time.Date(et.Year(), et.Month(), et.Day(), 0, 0, 0, 0, loc)
}

func SlotMinutes(timeframe Timeframe) (int, bool) {
	if timeframe == M1 {
		return 1, true
	}
	_, _, period, ok := timeframe.Aggregation()
	if !ok {
		return 0, false
	}
	minutes := int(period / time.Minute)
	if minutes < 1 || VolumeProfileSlots%minutes != 0 {
		return 0, false
	}
	return minutes, true
}

func DownsampleCumulative(cumulative []float32, slotMinutes int) []int64 {
	buckets := len(cumulative) / slotMinutes
	result := make([]int64, buckets)
	for bucket := range result {
		result[bucket] = int64(cumulative[(bucket+1)*slotMinutes-1] + 0.5)
	}
	return result
}
