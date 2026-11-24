package main

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
)

const (
	liveHealInterval       = 1 * time.Minute
	muteChannelHealEvery   = 5 * time.Minute
	muteChannelHealPerPass = 3
	orphanReattachPerPass  = 500
)

// StartLiveHealLoop reengancha simbolos huerfanos y recicla canales dxLink
// mudos en horario extendido.
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
				now := time.Now()
				healer.ReattachOrphans(ctx, now, orphanReattachPerPass)
				if now.Sub(lastChannelHeal) >= muteChannelHealEvery {
					lastChannelHeal = now
					healer.HealMuteChannels(ctx, now, muteChannelHealPerPass)
				}
			}
		}
	}()
}
