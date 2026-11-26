package out

import (
	"context"
	"time"
)

type LiveHealer interface {
	HealMuteChannels(ctx context.Context, now time.Time, limit int) int
	ReattachOrphans(ctx context.Context, now time.Time, limit int) int
	HealSilentSymbols(ctx context.Context, now time.Time, limit int) int
	LogLiveHealth(now time.Time)
}
