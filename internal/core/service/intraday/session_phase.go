package intraday

import "time"

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
	minutes := et.Hour()*60 + et.Minute()
	switch {
	case minutes >= 4*60 && minutes < 9*60+30:
		return PhasePreMarket
	case minutes >= 9*60+30 && minutes < 16*60:
		return PhaseRegular
	case minutes >= 16*60 && minutes < 20*60:
		return PhasePostMarket
	default:
		return PhaseClosed
	}
}
