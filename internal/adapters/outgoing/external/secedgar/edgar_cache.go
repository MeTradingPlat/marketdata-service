package secedgar

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"
)

const companyFactsDownloadTimeout = 40 * time.Minute

var downloadCompanyFacts = downloadToFile

func (c *EdgarClient) ensureCachedZip(ctx context.Context) (string, error) {
	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("creating sec edgar cache dir: %w", err)
	}
	target := filepath.Join(c.cacheDir, "companyfacts_"+time.Now().Format("2006-01-02")+".zip")
	if info, err := os.Stat(target); err == nil && info.Size() > 0 {
		return target, nil
	}

	dlCtx, cancel := context.WithTimeout(ctx, companyFactsDownloadTimeout)
	defer cancel()
	if err := downloadCompanyFacts(dlCtx, bulkFactsURL, target); err != nil {
		if stale, ok := newestCachedCompanyFactsZip(c.cacheDir); ok {
			log.Warn().Err(err).Str("file", stale).Msg("using stale companyfacts.zip")
			return stale, nil
		}
		return "", err
	}
	cleanupOldCacheFiles(c.cacheDir, "companyfacts_", target)
	return target, nil
}
