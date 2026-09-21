package catchup

import (
	"context"
	"fmt"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

const (
	volumeProfileBatchSize  = 200
	volumeProfileBatchPause = 200 * time.Millisecond
)

func RefreshVolumeProfile(ctx context.Context, repo out.VolumeProfileRepository, windowStart, now time.Time) error {
	stale, err := repo.GetSymbolsWithStaleVolumeProfile(ctx, windowStart)
	if err != nil {
		return err
	}
	if len(stale) == 0 {
		log.Info().Msg("volume profile refresh: all symbols already done for this maintenance window, skipping")
		return nil
	}
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return fmt.Errorf("loading New York timezone: %w", err)
	}
	before := domain.StartOfDayET(now, loc)

	start := time.Now()
	found := 0
	for from := 0; from < len(stale); from += volumeProfileBatchSize {
		if err := ctx.Err(); err != nil {
			return err
		}
		chunk := stale[from:min(from+volumeProfileBatchSize, len(stale))]
		profiles, err := repo.ComputeProfiles(ctx, chunk, before)
		if err != nil {
			return fmt.Errorf("computing volume profiles: %w", err)
		}
		attemptedOnly := make([]string, 0)
		for _, symbol := range chunk {
			if _, ok := profiles[symbol]; !ok {
				attemptedOnly = append(attemptedOnly, symbol)
			}
		}
		if err := repo.SaveBatch(ctx, profiles, attemptedOnly); err != nil {
			return err
		}
		found += len(profiles)
		time.Sleep(volumeProfileBatchPause)
	}
	log.Info().Int("symbols", len(stale)).Int("with_profile", found).Dur("elapsed", time.Since(start)).Msg("volume profile refresh finished")
	return nil
}
