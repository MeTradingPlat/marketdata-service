package secedgar

import (
	"context"
	"fmt"
	"time"
)

func downloadInsiderQuarter(ctx context.Context, templates []string, year, quarter int, target string) error {
	var lastErr error
	for _, t := range templates {
		dlCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		lastErr = downloadToFile(dlCtx, fmt.Sprintf(t, year, quarter), target)
		cancel()
		if lastErr == nil {
			return nil
		}
	}
	return lastErr
}
