package secedgar

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func (c *EdgarClient) ensureCachedZip(ctx context.Context) (string, error) {
	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("creating sec edgar cache dir: %w", err)
	}
	target := filepath.Join(c.cacheDir, "companyfacts_"+time.Now().Format("2006-01-02")+".zip")
	if info, err := os.Stat(target); err == nil && info.Size() > 0 {
		return target, nil
	}

	dlCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()
	if err := downloadToFile(dlCtx, bulkFactsURL, target); err != nil {
		return "", err
	}
	cleanupOldCacheFiles(c.cacheDir, "companyfacts_", target)
	return target, nil
}
