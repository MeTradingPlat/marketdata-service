package main

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/intraday"
	"github.com/rs/zerolog/log"
)

const dayVolumeRefreshInterval = 5 * time.Minute

// StartDayVolumeRefreshLoop mantiene DayVolumeTracker al dia con el volumen
// real (Trade.dayVolume) del universo rastreado -- solo mientras el mercado
// puede estar activo (4am-8pm ET, dias habiles), igual que
// StartTradingStatusLoop: fuera de esa ventana el volumen no cambia y no
// vale la pena gastar sesiones DxLink por las dudas.
func StartDayVolumeRefreshLoop(ctx context.Context, gateway out.DayVolumeGateway, symbols out.SymbolRepository, tracker *intraday.DayVolumeTracker) {
	go func() {
		ticker := time.NewTicker(dayVolumeRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !isDayVolumeRefreshWindow(time.Now()) {
					continue
				}
				refreshDayVolumes(ctx, gateway, symbols, tracker)
			}
		}
	}()
}

func refreshDayVolumes(ctx context.Context, gateway out.DayVolumeGateway, symbols out.SymbolRepository, tracker *intraday.DayVolumeTracker) {
	tracked, err := symbols.Tracked(ctx)
	if err != nil {
		log.Error().Err(err).Msg("day volume refresh: failed to list tracked symbols")
		return
	}
	syms := make([]string, len(tracked))
	for i, s := range tracked {
		syms[i] = s.Symbol
	}

	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		log.Error().Err(err).Msg("day volume refresh: loading America/New_York failed")
		return
	}
	nowET := time.Now().In(loc)
	day := time.Date(nowET.Year(), nowET.Month(), nowET.Day(), 0, 0, 0, 0, loc)

	start := time.Now()
	volumes := gateway.FetchDayVolumes(ctx, syms)
	tracker.Update(day, volumes)
	log.Info().Int("requested", len(syms)).Int("resolved", len(volumes)).Dur("elapsed", time.Since(start)).
		Msg("day volume refresh finished")
}

func isDayVolumeRefreshWindow(now time.Time) bool {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return false
	}
	nowET := now.In(loc)
	if nowET.Weekday() == time.Saturday || nowET.Weekday() == time.Sunday {
		return false
	}
	hour := nowET.Hour()
	return hour >= 4 && hour < 20
}
