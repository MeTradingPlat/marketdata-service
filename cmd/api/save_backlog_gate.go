package main

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/rs/zerolog/log"
)

const liveRolloutBacklogLimit = 100000
const saveBacklogPollInterval = 500 * time.Millisecond
const saveBacklogWaitTimeout = 20 * time.Minute

func waitForSaveBacklog(ctx context.Context, ingest in.IngestCandlesService) {
	waitForSaveBacklogWithTimeout(ctx, ingest, saveBacklogWaitTimeout, saveBacklogPollInterval)
}

func waitForSaveBacklogWithTimeout(ctx context.Context, ingest in.IngestCandlesService, timeout, pollInterval time.Duration) bool {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	deadline := time.Now().Add(timeout)
	for ingest.PendingSaves() > liveRolloutBacklogLimit {
		if time.Now().After(deadline) {
			log.Error().
				Int("pending", ingest.PendingSaves()).
				Int("limit", liveRolloutBacklogLimit).
				Dur("timeout", timeout).
				Msg("save backlog wait timed out, continuing live M1 rollout")
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
	return false
}
