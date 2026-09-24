package main

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/intraday"
	"github.com/rs/zerolog/log"
)

const dayVolumeRefreshInterval = 5 * time.Minute

// StartDayVolumeRefreshLoop siembra DayVolumeTracker desde Postgres al
// arrancar (sobrevive un reinicio: sin esto, el filtro VOLUME cae al
// fallback de siempre -- la suma de velas, solo 40-60% del real -- durante
// hasta 5 min en cada deploy) y despues lo mantiene al dia con el volumen
// real (Trade.dayVolume) del universo rastreado, guardando cada ronda en
// Postgres -- solo mientras el mercado puede estar activo (4am-8pm ET, dias
// habiles), igual que StartTradingStatusLoop: fuera de esa ventana el
// volumen no cambia y no vale la pena gastar sesiones DxLink por las dudas.
func StartDayVolumeRefreshLoop(ctx context.Context, gateway out.DayVolumeGateway, repo out.DayVolumeRepository, symbols out.SymbolRepository, tracker *intraday.DayVolumeTracker) {
	seedDayVolumesFromDB(ctx, repo, symbols, tracker)
	StartDayVolumeBoundaryLoop(ctx, gateway, repo, symbols, tracker)
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
				refreshDayVolumes(ctx, gateway, repo, symbols, tracker)
			}
		}
	}()
}

func seedDayVolumesFromDB(ctx context.Context, repo out.DayVolumeRepository, symbols out.SymbolRepository, tracker *intraday.DayVolumeTracker) {
	syms, day, err := trackedSymbolsAndDayET(ctx, symbols)
	if err != nil {
		log.Error().Err(err).Msg("day volume seed: failed to list tracked symbols")
		return
	}
	volumes, err := repo.GetBatch(ctx, syms, day)
	if err != nil {
		log.Error().Err(err).Msg("day volume seed: failed to load from db")
		return
	}
	tracker.Update(day, volumes)
	if boundaries, err := repo.GetBoundaries(ctx, syms, day); err != nil {
		log.Error().Err(err).Msg("day volume seed: failed to load boundaries from db")
	} else {
		tracker.SetBoundary(day, intraday.PreMarketEnd, boundaries.PreMarketEnd)
		tracker.SetBoundary(day, intraday.RegularEnd, boundaries.RegularEnd)
	}
	log.Info().Int("symbols", len(volumes)).Msg("day volume tracker seeded from db")
}

func refreshDayVolumes(ctx context.Context, gateway out.DayVolumeGateway, repo out.DayVolumeRepository, symbols out.SymbolRepository, tracker *intraday.DayVolumeTracker) {
	syms, day, err := trackedSymbolsAndDayET(ctx, symbols)
	if err != nil {
		log.Error().Err(err).Msg("day volume refresh: failed to list tracked symbols")
		return
	}

	start := time.Now()
	dayVolumeFetchMu.Lock()
	volumes := gateway.FetchDayVolumes(ctx, syms)
	dayVolumeFetchMu.Unlock()
	tracker.Update(day, volumes)
	if err := repo.SaveBatch(ctx, day, volumes); err != nil {
		log.Error().Err(err).Msg("day volume refresh: failed to save to db")
	}
	log.Info().Int("requested", len(syms)).Int("resolved", len(volumes)).Dur("elapsed", time.Since(start)).
		Msg("day volume refresh finished")
}

func trackedSymbolsAndDayET(ctx context.Context, symbols out.SymbolRepository) ([]string, time.Time, error) {
	tracked, err := symbols.Tracked(ctx)
	if err != nil {
		return nil, time.Time{}, err
	}
	syms := make([]string, len(tracked))
	for i, s := range tracked {
		syms[i] = s.Symbol
	}

	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return nil, time.Time{}, err
	}
	nowET := time.Now().In(loc)
	day := time.Date(nowET.Year(), nowET.Month(), nowET.Day(), 0, 0, 0, 0, loc)
	return syms, day, nil
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
