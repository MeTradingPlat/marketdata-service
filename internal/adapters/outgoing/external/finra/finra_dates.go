package finra

import (
	"sort"
	"time"
)

// recentSettlementDates genera las fechas candidatas de settlement quincenal
// (mitad de mes + fin de mes) de los ultimos `count` meses, mas recientes
// primero -- FINRA no publica en un dia fijo exacto, asi que probamos
// varias hasta que una exista.
//
// El settlement de mediados de mes es "el 15, o el dia habil ANTERIOR si
// el 15 cae en fin de semana/feriado" (calendario oficial de FINRA, Rule
// 4560). Ajustar hacia ADELANTE generaba el 17 en vez del 14 -- y cuando
// el archivo del 14 se publica (ej. shrt20260814.csv), probar el 17 y
// fallar dejaba al descargador en el archivo del mes ANTERIOR (confirmado
// contra la lista oficial de archivos de FINRA). Hacia atras coincide
// exactamente con el nombre de archivo que FINRA publica.
func recentSettlementDates(count int) []time.Time {
	seen := make(map[string]bool)
	var dates []time.Time
	today := time.Now()
	cursor := today

	for monthsBack := 0; monthsBack < count; monthsBack++ {
		midDay := 15
		if daysInMonth(cursor) < midDay {
			midDay = daysInMonth(cursor)
		}
		midMonth := nearestBusinessDay(time.Date(cursor.Year(), cursor.Month(), midDay, 0, 0, 0, 0, time.UTC), true)
		endMonth := lastBusinessDayOfMonth(cursor)

		if !midMonth.After(today) {
			addUnique(&dates, seen, midMonth)
		}
		if !endMonth.After(today) {
			addUnique(&dates, seen, endMonth)
		}
		cursor = time.Date(cursor.Year(), cursor.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
	}

	sort.Slice(dates, func(i, j int) bool { return dates[i].After(dates[j]) })
	if len(dates) > count {
		dates = dates[:count]
	}
	return dates
}

func daysInMonth(t time.Time) int {
	return time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func lastBusinessDayOfMonth(t time.Time) time.Time {
	last := time.Date(t.Year(), t.Month(), daysInMonth(t), 0, 0, 0, 0, time.UTC)
	return nearestBusinessDay(last, true)
}

func nearestBusinessDay(t time.Time, backwards bool) time.Time {
	for t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		if backwards {
			t = t.AddDate(0, 0, -1)
		} else {
			t = t.AddDate(0, 0, 1)
		}
	}
	return t
}

func addUnique(dates *[]time.Time, seen map[string]bool, t time.Time) {
	key := t.Format("2006-01-02")
	if !seen[key] {
		seen[key] = true
		*dates = append(*dates, t)
	}
}
