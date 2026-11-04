package main

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
)

const liveRolloutBacklogLimit = 100000
const saveBacklogPollInterval = 500 * time.Millisecond

func waitForSaveBacklog(ctx context.Context, ingest in.IngestCandlesService) {
	ticker := time.NewTicker(saveBacklogPollInterval)
	defer ticker.Stop()
	for ingest.PendingSaves() > liveRolloutBacklogLimit {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
