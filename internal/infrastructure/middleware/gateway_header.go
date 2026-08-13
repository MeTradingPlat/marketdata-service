package middleware

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

const gatewayHeader = "X-Gateway-Passed"

func GatewayHeaderCheck(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if isExemptPath(c.Request().URL.Path) {
			return next(c)
		}
		if c.Request().Header.Get(gatewayHeader) != "true" {
			return echo.NewHTTPError(http.StatusForbidden, "missing X-Gateway-Passed header")
		}
		return next(c)
	}
}

func isExemptPath(path string) bool {
	return path == "/marketdata/health"
}
