package secedgar

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

type yearQuarter struct {
	year    int
	quarter int
}

func (c *InsiderOwnershipClient) ensureCachedZips(ctx context.Context, lookbackQuarters int) ([]string, error) {
	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating sec edgar cache dir: %w", err)
	}

	var zipFiles []string
	for _, q := range recentQuarters(lookbackQuarters) {
		target := filepath.Join(c.cacheDir, fmt.Sprintf("insider_%dq%d.zip", q.year, q.quarter))
		if info, err := os.Stat(target); err == nil && info.Size() > 0 {
			zipFiles = append(zipFiles, target)
			continue
		}

		if err := downloadInsiderQuarter(ctx, insiderZipURLTemplates, q.year, q.quarter, target); err != nil {
			log.Warn().Err(err).Int("year", q.year).Int("quarter", q.quarter).Msg("insider zip unavailable")
			continue
		}
		zipFiles = append(zipFiles, target)
	}
	if len(zipFiles) == 0 {
		return nil, fmt.Errorf("no insider ownership zip available for the last %d quarters", lookbackQuarters)
	}
	cleanupInsiderCacheFiles(c.cacheDir, zipFiles)
	return zipFiles, nil
}

func recentQuarters(count int) []yearQuarter {
	now := time.Now()
	year := now.Year()
	quarter := (int(now.Month())-1)/3 + 1

	quarters := make([]yearQuarter, 0, count)
	for i := 0; i < count; i++ {
		quarters = append(quarters, yearQuarter{year, quarter})
		quarter--
		if quarter == 0 {
			quarter = 4
			year--
		}
	}
	return quarters
}

func cleanupInsiderCacheFiles(cacheDir string, keep []string) {
	keepSet := make(map[string]bool, len(keep))
	for _, k := range keep {
		keepSet[k] = true
	}
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		full := filepath.Join(cacheDir, e.Name())
		if strings.HasPrefix(e.Name(), "insider_") && !keepSet[full] {
			os.Remove(full)
		}
	}
}
