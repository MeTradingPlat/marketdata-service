package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type ConnectionStatus interface {
	Connected() bool
}

type HealthHandler struct {
	dxlink ConnectionStatus
}

func NewHealthHandler(dxlink ConnectionStatus) *HealthHandler {
	return &HealthHandler{dxlink: dxlink}
}

func (h *HealthHandler) Health(c echo.Context) error {
	status := "UP"
	code := http.StatusOK
	if !h.dxlink.Connected() {
		status = "DOWN"
		code = http.StatusServiceUnavailable
	}
	return c.JSON(code, map[string]string{"status": status})
}
