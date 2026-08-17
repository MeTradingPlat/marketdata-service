package finra

import (
	"testing"
	"time"
)

func candidateStrings(dates []time.Time) []string {
	out := make([]string, len(dates))
	for i, d := range dates {
		out[i] = d.Format("2006-01-02")
	}
	return out
}

func containsDate(dates []string, want string) bool {
	for _, d := range dates {
		if d == want {
			return true
		}
	}
	return false
}

func TestRecentSettlementDatesMidMonthWeekend(t *testing.T) {
	// 2026-08-15 cae sabado -- el settlement real de FINRA es el viernes 14.
	// Si la lista contiene el 17 (ajuste hacia adelante) pero no el 14, el
	// descargador pierde el archivo mas nuevo cuando FINRA lo publica.
	dates := candidateStrings(recentSettlementDates(6))
	if !containsDate(dates, "2026-08-14") {
		t.Errorf("falta 2026-08-14 (viernes anterior al 15 sabado): %v", dates)
	}
	if containsDate(dates, "2026-08-17") {
		t.Errorf("contiene 2026-08-17 (lunes posterior) que FINRA nunca usa: %v", dates)
	}
}

func TestRecentSettlementDatesMidMonthWeekday(t *testing.T) {
	// 2026-07-15 cae miercoles -- se usa el 15 tal cual.
	dates := candidateStrings(recentSettlementDates(6))
	if !containsDate(dates, "2026-07-15") {
		t.Errorf("falta 2026-07-15 (miercoles): %v", dates)
	}
}

func TestRecentSettlementDatesEndOfMonth(t *testing.T) {
	// 2026-07-31 cae viernes -- ultimo dia habil del mes tal cual.
	dates := candidateStrings(recentSettlementDates(6))
	if !containsDate(dates, "2026-07-31") {
		t.Errorf("falta 2026-07-31 (ultimo dia habil): %v", dates)
	}
}

func TestRecentSettlementDatesDescending(t *testing.T) {
	dates := recentSettlementDates(6)
	for i := 1; i < len(dates); i++ {
		if !dates[i-1].After(dates[i]) {
			t.Errorf("no ordenadas descendentes en %d: %v", i, dates)
		}
	}
}
