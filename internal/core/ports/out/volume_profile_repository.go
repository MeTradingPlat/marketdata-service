package out

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

type VolumeProfileRepository interface {
	GetSymbolsWithStaleVolumeProfile(ctx context.Context, windowStart time.Time) ([]string, error)
	ComputeProfiles(ctx context.Context, symbols []string, before time.Time) (map[string]domain.VolumeProfile, error)
	SaveBatch(ctx context.Context, profiles map[string]domain.VolumeProfile, attemptedOnly []string) error
	GetBatch(ctx context.Context, symbols []string) (map[string]domain.VolumeProfile, error)
}
