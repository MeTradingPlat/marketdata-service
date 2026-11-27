package timescale

import (
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

type postMarketWindow struct {
	start     time.Time
	postStart time.Time
	end       time.Time
}

func previousPostMarketWindows(before time.Time, days int, loc *time.Location) []postMarketWindow {
	beforeET := before.In(loc)
	firstDay := time.Date(beforeET.Year(), beforeET.Month(), beforeET.Day(), 0, 0, 0, 0, loc)
	if beforeET.Before(domain.NYSESessionHoursFor(beforeET).PostEnd) {
		firstDay = firstDay.AddDate(0, 0, -1)
	}
	windows := make([]postMarketWindow, 0, days)
	for i := 0; i < days; i++ {
		d := firstDay.AddDate(0, 0, -i)
		h := domain.NYSESessionHoursFor(d)
		windows = append(windows, postMarketWindow{
			start:     h.PreOpen,
			postStart: h.Close,
			end:       h.PostEnd,
		})
	}
	return windows
}
