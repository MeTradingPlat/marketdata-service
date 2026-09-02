package main

import "time"

const (
	saveRetryBaseInterval  = 15 * time.Second
	saveRetryMaxInterval   = 2 * time.Minute
	saveRetryBackoffFactor = 2
)

// nextSaveRetryInterval decide cuanto esperar hasta el proximo intento de
// RetryPendingSaves. Un fallo duplica la espera (tope 2min); cualquier
// resultado sin fallo (exito o nada pendiente) vuelve al piso de 15s --
// confirmado en vivo el 2026-09-02: con Postgres ya degradado por una caida
// real, este mismo reintento a 15s fijos sin importar el estado de la BD
// sumaba mas carga justo en el peor momento, sin darle margen para
// recuperarse entre intentos.
func nextSaveRetryInterval(current time.Duration, succeeded bool) time.Duration {
	if succeeded {
		return saveRetryBaseInterval
	}
	next := current * saveRetryBackoffFactor
	if next > saveRetryMaxInterval {
		return saveRetryMaxInterval
	}
	return next
}
