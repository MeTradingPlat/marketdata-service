package handler

import (
	"errors"
	"net/http"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/volumeprofile"
	"github.com/labstack/echo/v4"
)

const maxVolumeProfileSymbols = 1000

type volumeProfileRequest struct {
	Symbols   []string `json:"symbols"`
	Timeframe string   `json:"timeframe"`
}

type VolumeProfileHandler struct {
	service in.GetVolumeProfilesService
}

func NewVolumeProfileHandler(service in.GetVolumeProfilesService) *VolumeProfileHandler {
	return &VolumeProfileHandler{service: service}
}

func (h *VolumeProfileHandler) GetVolumeProfiles(c echo.Context) error {
	var req volumeProfileRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if len(req.Symbols) > maxVolumeProfileSymbols {
		return echo.NewHTTPError(http.StatusBadRequest, "too many symbols")
	}
	profiles, err := h.service.GetVolumeProfiles(c.Request().Context(), req.Symbols, domain.Timeframe(req.Timeframe))
	if errors.Is(err, volumeprofile.ErrUnsupportedTimeframe) {
		return echo.NewHTTPError(http.StatusBadRequest, "timeframe without volume profile")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "loading volume profiles failed")
	}
	return c.JSON(http.StatusOK, dto.VolumeProfilesResponse{Profiles: profiles})
}
