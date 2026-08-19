package middleware

import (
	"net/http"
	"sync/atomic"

	"github.com/labstack/echo/v4"
)

// MaintenanceCode es el codigo que el frontend y signal-processing
// reconocen para mostrar "el servicio esta en mantenimiento, espere" --
// el gate la devuelve durante el backfill/refill del ciclo (D1 -> beta ->
// H1 -> M1 -> prevClose), cuando las velas se estan rellenando y leerlas
// daria datos a medio completar. El health queda abierto para la
// orquestacion del contenedor.
const MaintenanceCode = "MAINTENANCE"

// BackfillGate rechaza TODAS las peticiones durante el backfill con un
// cuerpo estructurado identificable (MaintenanceCode) -- decision de
// diseno del usuario el 2026-08-19: el frontend muestra "en
// mantenimiento, espere" y signal-processing reintenta con backoff, en
// vez del throttle parcial que dejaba respuestas confusas.
func BackfillGate(backfilling *atomic.Bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c.Path() == "/marketdata/health" {
				return next(c)
			}
			if backfilling.Load() {
				return c.JSON(http.StatusServiceUnavailable, map[string]string{
					"code":    MaintenanceCode,
					"message": "El servicio esta en mantenimiento (refill de datos), por favor espere e intente de nuevo en unos minutos.",
				})
			}
			return next(c)
		}
	}
}
