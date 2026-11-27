package timescale

import (
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

type sessionCloseWindow struct {
	from time.Time
	to   time.Time
}

func previousSessionCloseWindows(before time.Time, days int, loc *time.Location) []sessionCloseWindow {
	beforeET := before.In(loc)
	firstDay := time.Date(beforeET.Year(), beforeET.Month(), beforeET.Day(), 0, 0, 0, 0, loc)
	if beforeET.Before(domain.NYSESessionHoursFor(beforeET).PostEnd) {
		firstDay = firstDay.AddDate(0, 0, -1)
	}
	windows := make([]sessionCloseWindow, 0, days)
	for i := 0; i < days; i++ {
		d := firstDay.AddDate(0, 0, -i)
		closeAt := domain.NYSESessionHoursFor(d).Close
		// to = Close+1min: la ts=Close es la subasta de cierre, el minuto siguiente ya es after-hours.
		windows = append(windows, sessionCloseWindow{
			from: closeAt.Add(-2 * time.Minute),
			to:   closeAt.Add(time.Minute),
		})
	}
	return windows
}
