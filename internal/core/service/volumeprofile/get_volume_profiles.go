package volumeprofile

import (
	"context"
	"errors"
	"fmt"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
)

var ErrUnsupportedTimeframe = errors.New("timeframe sin perfil de volumen")

type getVolumeProfilesService struct {
	repo out.VolumeProfileRepository
}

func NewGetVolumeProfilesService(repo out.VolumeProfileRepository) in.GetVolumeProfilesService {
	return &getVolumeProfilesService{repo: repo}
}

func (s *getVolumeProfilesService) GetVolumeProfiles(ctx context.Context, symbols []string, timeframe domain.Timeframe) (map[string]dto.VolumeProfile, error) {
	slotMinutes, ok := domain.SlotMinutes(timeframe)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedTimeframe, timeframe)
	}
	profiles, err := s.repo.GetBatch(ctx, symbols)
	if err != nil {
		return nil, err
	}
	result := make(map[string]dto.VolumeProfile, len(profiles))
	for symbol, profile := range profiles {
		result[symbol] = dto.VolumeProfile{
			SlotMinutes: slotMinutes,
			Sessions:    profile.Sessions,
			Cumulative:  domain.DownsampleCumulative(profile.Cumulative, slotMinutes),
		}
	}
	return result, nil
}
