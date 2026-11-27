package domain

import "time"

type NYSESessionHours struct {
	PreOpen time.Time
	Open    time.Time
	Close   time.Time
	PostEnd time.Time
}

func NYSESessionHoursFor(dayET time.Time) NYSESessionHours {
	y, m, d := dayET.Date()
	loc := dayET.Location()
	closeHour, postEndHour := 16, 20
	if IsNYSEEarlyClose(dayET) {
		closeHour, postEndHour = 13, 17
	}
	return NYSESessionHours{
		PreOpen: time.Date(y, m, d, 4, 0, 0, 0, loc),
		Open:    time.Date(y, m, d, 9, 30, 0, 0, loc),
		Close:   time.Date(y, m, d, closeHour, 0, 0, 0, loc),
		PostEnd: time.Date(y, m, d, postEndHour, 0, 0, 0, loc),
	}
}
