package domain

import "time"

func IsNYSEEarlyClose(day time.Time) bool {
	wd := day.Weekday()
	if wd < time.Monday || wd > time.Friday {
		return false
	}
	_, m, d := day.Date()
	if m == time.November && wd == time.Friday {
		return d >= 23 && d <= 29
	}
	return wd <= time.Thursday && isEarlyCloseEve(m, d)
}

func isEarlyCloseEve(m time.Month, d int) bool {
	return (m == time.December && d == 24) || (m == time.July && d == 3)
}

func isGoodFriday(y int, m time.Month, d int) bool {
	em, ed := gregorianEaster(y)
	gf := time.Date(y, em, ed, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -2)
	return m == gf.Month() && d == gf.Day()
}

func gregorianEaster(y int) (time.Month, int) {
	a := y % 19
	b := y / 100
	c := y % 100
	d := b / 4
	e := b % 4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i := c / 4
	k := c % 4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := time.Month((h + l - 7*m + 114) / 31)
	day := ((h + l - 7*m + 114) % 31) + 1
	return month, day
}
