package domain

import "math"

// maxPlausibleShortInterestPct: por encima de esto, sharesShorted/float
// casi siempre significa float mal calculado (o sharesShorted de una
// clase distinta) y no que el 300%+ del float este realmente en corto --
// se prefiere omitir el dato a mostrar un numero absurdo.
const maxPlausibleShortInterestPct = 300.0

// ShortInterestPercent calcula el % del float en corto al momento de
// LEER, no al guardar -- el float puede corregirse despues (ver el
// refresco de holders 13D), y un porcentaje persistido quedaria
// desincronizado para siempre.
func ShortInterestPercent(sharesShorted *int64, floatShares *int64) *float64 {
	if sharesShorted == nil || floatShares == nil || *sharesShorted <= 0 || *floatShares <= 0 {
		return nil
	}
	pct := math.Round(float64(*sharesShorted)/float64(*floatShares)*100*100) / 100
	if pct > maxPlausibleShortInterestPct {
		return nil
	}
	return &pct
}
