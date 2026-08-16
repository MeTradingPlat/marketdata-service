package domain

import "time"

// LiveMarketCap escala el market-cap guardado (de /market-metrics, que
// TastyTrade solo recalcula de su lado unas pocas veces al dia, ver
// updated-at en la respuesta real) por cuanto se movio el precio desde
// entonces -- mismo principio que Java (recomputeMarketCapFromLivePrice),
// sin necesitar sharesOutstanding: si el precio subio 1%, el market cap
// tambien, sin importar cuantas acciones haya. Si no hay ancla real
// (PrevClose o CurrentPrice en 0, simbolo nuevo sin velas todavia) se sirve
// el valor crudo sin escalar en vez de dividir por cero. Usado tanto por
// /symbols/{symbol}/details como por /marketdata/fundamentals/realtime.
func LiveMarketCap(stored float64, snapshot IntradaySnapshot) float64 {
	if stored == 0 || snapshot.PrevClose == 0 || snapshot.CurrentPrice == 0 {
		return stored
	}
	return stored * (snapshot.CurrentPrice / snapshot.PrevClose)
}

// DaysUntil calcula dias restantes al vuelo en vez de guardarlo -- guardado
// se volveria stale al dia siguiente sin un refresco que lo recalcule.
func DaysUntil(isoDate string) int {
	if isoDate == "" {
		return 0
	}
	target, err := time.Parse("2006-01-02", isoDate)
	if err != nil {
		return 0
	}
	days := int(time.Until(target).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}
