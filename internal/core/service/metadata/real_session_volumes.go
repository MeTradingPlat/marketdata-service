package metadata

import (
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/intraday"
)

func withRealSessionVolumes(tracker *intraday.DayVolumeTracker, symbol string, snapshot domain.IntradaySnapshot, now time.Time) domain.IntradaySnapshot {
	current, known := tracker.Get(symbol)
	if !known {
		return snapshot
	}
	phase := intraday.PhaseAt(now)
	if phase == intraday.PhasePreMarket {
		snapshot.PreMarketVolume = current
		return snapshot
	}
	if preMarket, ok := tracker.Boundary(symbol, intraday.PreMarketEnd); ok && phase != intraday.PhaseClosed {
		snapshot.PreMarketVolume = preMarket
	}
	if regularEnd, ok := tracker.Boundary(symbol, intraday.RegularEnd); ok && phase == intraday.PhasePostMarket {
		snapshot.PostMarketVolume = max(current-regularEnd, 0)
	}
	return snapshot
}
