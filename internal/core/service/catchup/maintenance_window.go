package catchup

import (
	"time"
)

const (
	postCloseDelay = 5 * time.Minute
	// marketCloseHourET es el cierre del POST-MARKET extendido, 20:00 ET (beta necesita el cierre real con post-market).
	marketCloseHourET = 20
)

// LastMaintenanceWindowStart es el arranque de la ventana de mantenimiento
// mas reciente (cierre + postCloseDelay) -- la frontera que separa "lo
// calculado en esta ventana" de "lo calculado antes": un done_at de un
// refresh de fundamentales posterior a esta marca significa que el paso ya
// corrio tras el ultimo cierre y no hay que recalcularlo en un reinicio.
func LastMaintenanceWindowStart(now time.Time) time.Time {
	return NextMaintenanceWindowAt(now).Add(-24 * time.Hour)
}

// NextMaintenanceWindowAt es la proxima vez que el mercado (post-market
// extendido incluido) lleva postCloseDelay sin actividad -- se calcula en
// hora de Nueva York, no UTC. Con UTC fijo la ventana caia a medianoche UTC,
// que en horario estandar (EST, invierno) son las 7pm ET: todavia dentro
// del post-market real, compitiendo con RunSweep por las mismas conexiones
// DxLink en vez de esperar a que el mercado este de verdad inactivo (ver
// CandlePool.StopAllLive).
func NextMaintenanceWindowAt(now time.Time) time.Time {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		next := midnight.Add(postCloseDelay)
		if !next.After(now) {
			next = next.Add(24 * time.Hour)
		}
		return next
	}

	nowET := now.In(loc)
	target := time.Date(nowET.Year(), nowET.Month(), nowET.Day(), marketCloseHourET, 0, 0, 0, loc).Add(postCloseDelay)
	if !target.After(nowET) {
		target = target.AddDate(0, 0, 1)
	}
	return target
}
