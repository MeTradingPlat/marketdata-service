package main

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
)

const (
	liveHealInterval       = 1 * time.Minute
	liveHealCallTimeout    = 3 * time.Minute
	muteChannelHealEvery   = 5 * time.Minute
	muteChannelHealPerPass = 3
	orphanReattachPerPass  = 500
	silentHealPerPass      = 100
)

func StartLiveHealLoop(ctx context.Context, healer out.LiveHealer) {
	go func() {
		ticker := time.NewTicker(liveHealInterval)
		defer ticker.Stop()
		var lastChannelHeal time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runLiveHealTick(ctx, healer, time.Now(), &lastChannelHeal)
			}
		}
	}()
}

func runLiveHealTick(ctx context.Context, healer out.LiveHealer, now time.Time, lastChannelHeal *time.Time) {
	healer.LogLiveHealth(now)
	runWithHealTimeout(ctx, func(c context.Context) { healer.HealSilentSymbols(c, now, silentHealPerPass) })
	runWithHealTimeout(ctx, func(c context.Context) { healer.ReattachOrphans(c, now, orphanReattachPerPass) })
	if now.Sub(*lastChannelHeal) >= muteChannelHealEvery {
		*lastChannelHeal = now
		runWithHealTimeout(ctx, func(c context.Context) { healer.HealMuteChannels(c, now, muteChannelHealPerPass) })
	}
}

func runWithHealTimeout(parent context.Context, fn func(context.Context)) {
	ctx, cancel := context.WithTimeout(parent, liveHealCallTimeout)
	defer cancel()
	fn(ctx)
}
