package middleware

import (
	"net/http"
	"sync/atomic"

	"github.com/labstack/echo/v4"
)

// BackfillGate bloquea las peticiones HTTP mientras el servicio esta en la
// fase de backfill del ciclo (D1 -> beta/prevClose -> H1 -> M1 -> gap
// check): las estrategias y el frontend no deben leer velas a medio
// rellenar (ver universe_cycle.go, backfilling se prende al arrancar el
// ciclo y se apaga al terminar el rollout M1 y su verificacion). El
// health queda abierto para la orquestacion del contenedor.
func BackfillGate(backfilling *atomic.Bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c.Path() == "/marketdata/health" {
				return next(c)
			}
			if backfilling.Load() {
				return c.JSON(http.StatusServiceUnavailable, map[string]string{"message": "service backfilling candles, retry later"})
			}
			return next(c)
		}
	}
}
