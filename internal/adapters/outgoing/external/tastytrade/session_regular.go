package tastytrade

import "time"

func isRegularSessionET(now time.Time) bool {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return false
	}
	nowET := now.In(loc)
	if nowET.Weekday() == time.Saturday || nowET.Weekday() == time.Sunday {
		return false
	}
	minutes := nowET.Hour()*60 + nowET.Minute()
	return minutes >= 9*60+30 && minutes < 16*60
}
