package domain

import "time"

func IsNYSEHoliday(day time.Time) bool {
	wd := day.Weekday()
	if wd == time.Saturday || wd == time.Sunday {
		return false
	}
	y, m, d := day.Date()
	return isFixedNYSEHoliday(y, m, d, wd) || isFloatingNYSEHoliday(y, m, d, wd)
}

func isFixedNYSEHoliday(y int, m time.Month, d int, wd time.Weekday) bool {
	if m == time.January && (d == 1 || (d == 2 && wd == time.Monday)) {
		return true
	}
	if y >= 2022 && isObservedFixedDay(m, d, wd, time.June, 19) {
		return true
	}
	return isObservedFixedDay(m, d, wd, time.July, 4) || isObservedFixedDay(m, d, wd, time.December, 25)
}

func isObservedFixedDay(m time.Month, d int, wd time.Weekday, targetM time.Month, targetD int) bool {
	if m != targetM {
		return false
	}
	return d == targetD || (d == targetD-1 && wd == time.Friday) || (d == targetD+1 && wd == time.Monday)
}

func isFloatingNYSEHoliday(y int, m time.Month, d int, wd time.Weekday) bool {
	switch m {
	case time.January:
		return isNthWeekday(d, wd, time.Monday, 3)
	case time.February:
		return isNthWeekday(d, wd, time.Monday, 3)
	case time.May:
		return wd == time.Monday && d >= 25
	case time.September:
		return isNthWeekday(d, wd, time.Monday, 1)
	case time.November:
		return isNthWeekday(d, wd, time.Thursday, 4)
	default:
		return isGoodFriday(y, m, d)
	}
}

func isNthWeekday(d int, wd, target time.Weekday, n int) bool {
	return wd == target && (d-1)/7+1 == n
}
