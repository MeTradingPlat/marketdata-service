package middleware

import (
	"time"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

func RequestLogging(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		start := time.Now()
		err := next(c)
		log.Info().
			Str("method", c.Request().Method).
			Str("path", c.Request().URL.Path).
			Int("status", c.Response().Status).
			Dur("duration", time.Since(start)).
			Msg("request handled")
		return err
	}
}
