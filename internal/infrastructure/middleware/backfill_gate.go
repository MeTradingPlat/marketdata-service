package middleware

import (
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/labstack/echo/v4"
)

// MaintenanceCode es el codigo que signal-processing-service y el frontend
// reconocen para bajar la frecuencia de reintento en las lecturas pesadas
// que si llegan a rechazarse (ver heavyReadPaths).
const MaintenanceCode = "MAINTENANCE"

// lightReadPaths: rutas que SIEMPRE pasan, incluso durante el fill/refill --
// fundamentals y symbols/quotes ya se sirven desde SymbolsCache/
// FundamentalsCache (memoria, sin tocar Postgres salvo el fallback puntual
// de GetCurrentPrices para un simbolo sin tick en vivo, acotado por su
// propio semaforo). Diseño original del 2026-08-18 (ver
// docs/dxlink-session-incidents.md): un 503 total dejaba la app inutilizable
// 20+ min por ciclo -- se habia perdido en algun punto antes de esta fecha y
// se restaura el 2026-09-03 en vez del bloqueo total por ruta que se probo
// primero ese mismo dia.
var lightReadPaths = []string{
	"/marketdata/health",
	"/marketdata/symbols",
	"/marketdata/markets",
	"/marketdata/timeframes",
	"/marketdata/fundamentals",
	"/marketdata/quotes",
}

// heavyReadPaths: las lecturas de velas que durante el refill compiten de
// verdad por el pool de escritura/lectura de Postgres que el barrido
// necesita -- se limitan en concurrencia en vez de servirse siempre o
// bloquearse siempre.
var heavyReadPaths = []string{
	"/marketdata/historical",
	"/marketdata/intraday",
	"/marketdata/candles",
}

const (
	// refillConcurrency: cuantas lecturas pesadas simultaneas se permiten
	// durante el refill -- el resto del pool de la BD queda para el barrido.
	refillConcurrency = 4
	// refillThrottleWait: cuanto espera una peticion pesada por un slot
	// antes de responder 503 (el caller reintenta corto).
	refillThrottleWait = 1500 * time.Millisecond
)

// BackfillGate dejar pasar siempre las lecturas livianas durante el
// fill/refill y limita en concurrencia las pesadas -- el sistema sigue
// usable (screener, fundamentales, charts con retry corto) mientras el
// barrido conserva los recursos de la BD. Cualquier ruta no listada en
// ninguno de los dos grupos (WS, endpoints nuevos) tambien pasa siempre:
// solo las explicitamente pesadas se acotan.
func BackfillGate(backfilling *atomic.Bool) echo.MiddlewareFunc {
	slots := make(chan struct{}, refillConcurrency)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Path()
			if !backfilling.Load() || isLightRead(path) || !isHeavyRead(path) {
				return next(c)
			}
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
				return next(c)
			case <-time.After(refillThrottleWait):
				return c.JSON(http.StatusServiceUnavailable, map[string]string{
					"code":    MaintenanceCode,
					"message": "El servicio esta en mantenimiento (refill de datos), por favor espere e intente de nuevo en unos segundos.",
				})
			}
		}
	}
}

func isLightRead(path string) bool {
	for _, prefix := range lightReadPaths {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func isHeavyRead(path string) bool {
	for _, prefix := range heavyReadPaths {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
