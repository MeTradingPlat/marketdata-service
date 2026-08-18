package middleware

import (
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/labstack/echo/v4"
)

// lightReadPaths: los endpoints que SIEMPRE pasan, incluso durante el
// refill -- son lecturas ligeras (fundamentals, symbols, search) o de
// datos que el refill no toca (health para la orquestacion).
var lightReadPaths = []string{
	"/marketdata/health",
	"/marketdata/symbols",
	"/marketdata/markets",
	"/marketdata/timeframes",
	"/marketdata/fundamentals",
	"/marketdata/quotes",
}

// heavyReadPaths: las lecturas de velas que durante el refill se limitan
// en concurrencia -- el refill (sweeps, rollout, prevClose) tiene
// prioridad sobre los slots de la BD; las peticiones de charts esperan un
// slot o reciben 503 con retry corto. El dato que devuelven es correcto
// (watermark-driven: lo unico que puede faltar es la barra de hoy a mitad
// del fetch).
var heavyReadPaths = []string{
	"/marketdata/historical",
	"/marketdata/intraday",
	"/marketdata/candles",
}

const (
	// refillConcurrency: cuantas lecturas pesadas simultaneas se permiten
	// durante el refill -- el resto del pool de la BD queda para el refill.
	refillConcurrency = 4
	// refillThrottleWait: cuanto espera una peticion pesada por un slot
	// antes de responder 503 (el frontend reintenta).
	refillThrottleWait = 1500 * time.Millisecond
)

// BackfillGate reemplazo del 503 total: durante el refill las peticiones
// ligeras pasan siempre y las pesadas se limitan a refillConcurrency
// simultaneas -- el sistema sigue usable (screener, fundamentales, charts
// con retry corto) mientras el refill conserva los recursos de la BD.
// Confirmado en vivo el 2026-08-18: el 503 total dejo la app inutilizable
// 20+ min por ciclo; el throttle la mantiene viva sin sacrificar el refill.
func BackfillGate(backfilling *atomic.Bool) echo.MiddlewareFunc {
	slots := make(chan struct{}, refillConcurrency)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Path()
			if !backfilling.Load() || isLightRead(path) {
				return next(c)
			}
			if !isHeavyRead(path) {
				return next(c)
			}
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
				return next(c)
			case <-time.After(refillThrottleWait):
				return c.JSON(http.StatusServiceUnavailable, map[string]string{"message": "refill in progress, retry shortly"})
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
