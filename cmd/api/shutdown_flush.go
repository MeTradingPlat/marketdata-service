package main

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

const shutdownFlushTimeout = 8 * time.Second

type shutdownFlusher interface {
	FlushLiveSaves(ctx context.Context) bool
	RetryPendingSaves(ctx context.Context) bool
	PendingSaves() int
}

func flushSavesOnShutdown(flusher shutdownFlusher, limit time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	flusher.FlushLiveSaves(ctx)
	flusher.RetryPendingSaves(ctx)
	if pending := flusher.PendingSaves(); pending > 0 {
		log.Warn().Int("candles", pending).Msg("candles still pending at shutdown, the next M1 sweep refetches them from the watermark")
	}
}
