package metadata

import (
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/intraday"
)

var sessionDay = time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)

func etTime(hour, minute int) time.Time {
	loc, _ := time.LoadLocation("America/New_York")
	return time.Date(2026, 9, 23, hour, minute, 0, 0, loc)
}

func trackerWith(current, preMarketEnd, regularEnd int64) *intraday.DayVolumeTracker {
	tracker := intraday.NewDayVolumeTracker()
	tracker.Update(sessionDay, map[string]int64{"AAPL": current})
	if preMarketEnd > 0 {
		tracker.SetBoundary(sessionDay, intraday.PreMarketEnd, map[string]int64{"AAPL": preMarketEnd})
	}
	if regularEnd > 0 {
		tracker.SetBoundary(sessionDay, intraday.RegularEnd, map[string]int64{"AAPL": regularEnd})
	}
	return tracker
}

func candleBased() domain.IntradaySnapshot {
	return domain.IntradaySnapshot{PreMarketVolume: 11, PostMarketVolume: 22}
}

func TestRealSessionVolumes_DuringPreMarketEverythingTradedSoFarIsPreMarket(t *testing.T) {
	got := withRealSessionVolumes(trackerWith(500_000, 0, 0), "AAPL", candleBased(), etTime(8, 15))

	if got.PreMarketVolume != 500_000 || got.PostMarketVolume != 22 {
		t.Fatalf("pre=%d post=%d, want 500000 and the untouched 22", got.PreMarketVolume, got.PostMarketVolume)
	}
}

func TestRealSessionVolumes_AfterTheOpenPreMarketIsTheFrozenBoundary(t *testing.T) {
	got := withRealSessionVolumes(trackerWith(9_000_000, 480_000, 0), "AAPL", candleBased(), etTime(11, 0))

	if got.PreMarketVolume != 480_000 {
		t.Fatalf("pre = %d, want 480000", got.PreMarketVolume)
	}
}

func TestRealSessionVolumes_PostMarketIsWhatTradedAfterTheRegularClose(t *testing.T) {
	got := withRealSessionVolumes(trackerWith(20_300_000, 480_000, 20_000_000), "AAPL", candleBased(), etTime(17, 0))

	if got.PostMarketVolume != 300_000 || got.PreMarketVolume != 480_000 {
		t.Fatalf("pre=%d post=%d, want 480000 and 300000", got.PreMarketVolume, got.PostMarketVolume)
	}
}

func TestRealSessionVolumes_WithoutABoundaryTheCandleBasedValueStays(t *testing.T) {
	got := withRealSessionVolumes(trackerWith(9_000_000, 0, 0), "AAPL", candleBased(), etTime(17, 0))

	if got.PreMarketVolume != 11 || got.PostMarketVolume != 22 {
		t.Fatalf("pre=%d post=%d, want the candle-based 11 and 22", got.PreMarketVolume, got.PostMarketVolume)
	}
}

func TestRealSessionVolumes_AnUnknownSymbolKeepsTheCandleBasedSnapshot(t *testing.T) {
	got := withRealSessionVolumes(intraday.NewDayVolumeTracker(), "AAPL", candleBased(), etTime(17, 0))

	if got != candleBased() {
		t.Fatalf("snapshot changed for an unknown symbol: %+v", got)
	}
}
