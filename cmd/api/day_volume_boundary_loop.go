package main

import (
	"context"
	"sync"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/intraday"
	"github.com/rs/zerolog/log"
)

const dayBoundaryCheckInterval = 15 * time.Second

var dayVolumeFetchMu sync.Mutex

type boundaryKey struct {
	day  string
	kind intraday.DayBoundary
}

func StartDayVolumeBoundaryLoop(ctx context.Context, gateway out.DayVolumeGateway, repo out.DayVolumeRepository, symbols out.SymbolRepository, tracker *intraday.DayVolumeTracker) {
	go func() {
		captured := map[boundaryKey]bool{}
		ticker := time.NewTicker(dayBoundaryCheckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				kind, due := dueDayBoundary(now)
				key := boundaryKey{day: now.In(easternTime()).Format("2006-01-02"), kind: kind}
				if !due || captured[key] {
					continue
				}
				captured[key] = true
				captureDayBoundary(ctx, gateway, repo, symbols, tracker, kind)
			}
		}
	}()
}

func dueDayBoundary(now time.Time) (intraday.DayBoundary, bool) {
	et := now.In(easternTime())
	if et.Weekday() == time.Saturday || et.Weekday() == time.Sunday {
		return 0, false
	}
	hours := domain.NYSESessionHoursFor(et)
	preEndStart := hours.Open.Add(-4 * time.Minute)
	preEndStop := hours.Open.Add(-2 * time.Minute)
	regEndStart := hours.Close.Add(time.Minute)
	regEndStop := hours.Close.Add(4 * time.Minute)
	switch {
	case !et.Before(preEndStart) && et.Before(preEndStop):
		return intraday.PreMarketEnd, true
	case !et.Before(regEndStart) && et.Before(regEndStop):
		return intraday.RegularEnd, true
	}
	return 0, false
}

func captureDayBoundary(ctx context.Context, gateway out.DayVolumeGateway, repo out.DayVolumeRepository, symbols out.SymbolRepository, tracker *intraday.DayVolumeTracker, kind intraday.DayBoundary) {
	syms, day, err := trackedSymbolsAndDayET(ctx, symbols)
	if err != nil {
		log.Error().Err(err).Msg("day volume boundary: failed to list tracked symbols")
		return
	}
	dayVolumeFetchMu.Lock()
	start := time.Now()
	volumes := gateway.FetchDayVolumes(out.WithFastFetch(ctx), syms)
	dayVolumeFetchMu.Unlock()

	if isBoundaryCaptureTooLate(kind, time.Now()) {
		log.Warn().Int("kind", int(kind)).Dur("elapsed", time.Since(start)).
			Msg("day volume boundary discarded: the capture ended after the session boundary, its values include the next session")
		return
	}
	tracker.SetBoundary(day, kind, volumes)
	save := repo.SavePreMarketEnd
	if kind == intraday.RegularEnd {
		save = repo.SaveRegularEnd
	}
	if err := save(ctx, day, volumes); err != nil {
		log.Error().Err(err).Msg("day volume boundary: failed to save to db")
	}
	log.Info().Int("kind", int(kind)).Int("requested", len(syms)).Int("resolved", len(volumes)).Dur("elapsed", time.Since(start)).
		Msg("day volume boundary captured")
}

func easternTime() *time.Location {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return time.FixedZone("EST", -5*3600)
	}
	return loc
}

func isBoundaryCaptureTooLate(kind intraday.DayBoundary, now time.Time) bool {
	if kind != intraday.PreMarketEnd {
		return false
	}
	et := now.In(easternTime())
	return et.Hour()*3600+et.Minute()*60+et.Second() >= 9*3600+30*60
}
