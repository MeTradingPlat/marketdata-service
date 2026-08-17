package domain

import (
	"sort"
	"time"
)

// earningsCycleNoiseFloor: deltas menores a esto entre dos reportes
// consecutivos se descartan del calculo de mediana -- correcciones o
// reportes duplicados en la fuente, no un ciclo real (ningun emisor
// reporta ganancias con menos de ~2 meses de diferencia).
const earningsCycleNoiseFloor = 60

// LastEarningsReport calcula la fecha del ultimo reporte real (con EPS ya
// publicado) y predice el proximo via la MEDIANA del ciclo entre reportes
// pasados -- mismo algoritmo que la version Java (mediana en vez de
// promedio para no dejarse arrastrar por un atraso puntual o un año
// bisiesto). occurredDate/predictedNext vuelven "" si no hay suficientes
// datos utilizables.
func LastEarningsReport(reports []EarningsReportItem) (occurredDate string, predictedNext string) {
	dates := make([]time.Time, 0, len(reports))
	for _, r := range reports {
		t, err := time.Parse("2006-01-02", r.OccurredDate)
		if err != nil {
			continue
		}
		dates = append(dates, t)
	}
	if len(dates) == 0 {
		return "", ""
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })

	last := dates[len(dates)-1]
	occurredDate = last.Format("2006-01-02")

	var deltas []int
	for i := 1; i < len(dates); i++ {
		days := int(dates[i].Sub(dates[i-1]).Hours() / 24)
		if days > earningsCycleNoiseFloor {
			deltas = append(deltas, days)
		}
	}
	if len(deltas) == 0 {
		return occurredDate, ""
	}

	sort.Ints(deltas)
	n := len(deltas)
	var median int
	if n%2 == 0 {
		median = (deltas[n/2-1] + deltas[n/2]) / 2
	} else {
		median = deltas[n/2]
	}
	return occurredDate, last.AddDate(0, 0, median).Format("2006-01-02")
}
