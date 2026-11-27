package intraday

import (
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

type SessionPhase int

const (
	PhaseClosed SessionPhase = iota
	PhasePreMarket
	PhaseRegular
	PhasePostMarket
)

var newYork = loadNewYork()

func loadNewYork() *time.Location {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return time.FixedZone("EST", -5*3600)
	}
	return loc
}

func PhaseAt(now time.Time) SessionPhase {
	et := now.In(newYork)
	if et.Weekday() == time.Saturday || et.Weekday() == time.Sunday {
		return PhaseClosed
	}
	h := domain.NYSESessionHoursFor(et)
	switch {
	case !et.Before(h.PreOpen) && et.Before(h.Open):
		return PhasePreMarket
	case !et.Before(h.Open) && et.Before(h.Close):
		return PhaseRegular
	case !et.Before(h.Close) && et.Before(h.PostEnd):
		return PhasePostMarket
	default:
		return PhaseClosed
	}
}
