package middleware

import (
	"net/http"
	"sync/atomic"

	"github.com/labstack/echo/v4"
)

// MaintenanceCode es el codigo que signal-processing-service reconoce para
// bajar la frecuencia de reintento -- ver comentario de BackfillGate.
const MaintenanceCode = "MAINTENANCE"

// BackfillGate rechaza peticiones durante el backfill/refill con un cuerpo
// estructurado identificable (MaintenanceCode) -- restaurado el 2026-09-03
// (existio antes, se habia sacado por completo horas antes ese mismo dia)
// pero esta vez aplicado SOLO a las rutas de signal-processing-service
// (quotes/rest, fundamentals/realtime, historical/batch, symbols), no de
// forma global: confirmado en vivo que el barrido D1+H1+M1 de un firstRun
// (todo redeploy dispara uno) corriendo a la vez que signal-processing
// pedia mas de 1 req/s en pleno horario de mercado agotaba los mismos
// recursos compartidos (pool de escritura, disco) que el barrido necesita.
// Las rutas que usa el frontend (graficos, busqueda, detalle de simbolo)
// quedan afuera del gate a proposito -- un usuario mirando la plataforma no
// deberia ver "en mantenimiento" por un barrido que no le afecta a el.
func BackfillGate(backfilling *atomic.Bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
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
