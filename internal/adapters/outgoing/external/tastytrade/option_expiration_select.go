package tastytrade

import (
	"strings"
	"time"
)

// frontMonthlyExpiration elige el vencimiento mensual ('Regular') futuro mas
// cercano con strikes; si el subyacente solo lista weeklies u otros tipos,
// cae al vencimiento futuro mas cercano con strikes.
func frontMonthlyExpiration(expirations []optionExpiration, now time.Time) (optionExpiration, bool) {
	var (
		bestRegular     optionExpiration
		bestRegularDate time.Time
		hasRegular      bool

		bestAny     optionExpiration
		bestAnyDate time.Time
		hasAny      bool
	)

	for _, exp := range expirations {
		if len(exp.Strikes) == 0 {
			continue
		}
		t, err := time.Parse("2006-01-02", exp.ExpirationDate)
		if err != nil || !t.After(now) {
			continue
		}

		if strings.EqualFold(exp.ExpirationType, "Regular") {
			if !hasRegular || t.Before(bestRegularDate) {
				bestRegular = exp
				bestRegularDate = t
				hasRegular = true
			}
		}
		if !hasAny || t.Before(bestAnyDate) {
			bestAny = exp
			bestAnyDate = t
			hasAny = true
		}
	}

	if hasRegular {
		return bestRegular, true
	}
	if hasAny {
		return bestAny, true
	}
	return optionExpiration{}, false
}
