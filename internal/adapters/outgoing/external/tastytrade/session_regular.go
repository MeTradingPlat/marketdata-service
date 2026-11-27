package tastytrade

import (
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func nyseTimeET(now time.Time) (time.Time, bool) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return time.Time{}, false
	}
	nowET := now.In(loc)
	if nowET.Weekday() == time.Saturday || nowET.Weekday() == time.Sunday || domain.IsNYSEHoliday(nowET) {
		return time.Time{}, false
	}
	return nowET, true
}

func isRegularSessionET(now time.Time) bool {
	nowET, ok := nyseTimeET(now)
	if !ok {
		return false
	}
	closeHour := 16
	if domain.IsNYSEEarlyClose(nowET) {
		closeHour = 13
	}
	minutes := nowET.Hour()*60 + nowET.Minute()
	return minutes >= 9*60+30 && minutes < closeHour*60
}
