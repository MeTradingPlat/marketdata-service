package metadata

import (
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/intraday"
)

func TestWithRealDayVolume_UsesTheTrackedValueWhenPresent(t *testing.T) {
	tracker := intraday.NewDayVolumeTracker()
	tracker.Update(time.Now(), map[string]int64{"SPY": 5_468_232})
	snapshot := domain.IntradaySnapshot{Symbol: "SPY", DayVolume: 1_990_674}

	got := withRealDayVolume(tracker, "SPY", snapshot)

	if got.DayVolume != 5_468_232 {
		t.Fatalf("DayVolume = %d, want the real tracked value 5468232", got.DayVolume)
	}
}

func TestWithRealDayVolume_FallsBackToTheSnapshotWhenNotTrackedYet(t *testing.T) {
	tracker := intraday.NewDayVolumeTracker()
	snapshot := domain.IntradaySnapshot{Symbol: "MDXH", DayVolume: 42}

	got := withRealDayVolume(tracker, "MDXH", snapshot)

	if got.DayVolume != 42 {
		t.Fatalf("DayVolume = %d, want the candle-summed fallback 42", got.DayVolume)
	}
}

func TestWithRealDayVolume_LeavesTheRestOfTheSnapshotUntouched(t *testing.T) {
	tracker := intraday.NewDayVolumeTracker()
	tracker.Update(time.Now(), map[string]int64{"SPY": 999})
	snapshot := domain.IntradaySnapshot{Symbol: "SPY", DayVolume: 1, PreMarketVolume: 10, PostMarketVolume: 20, Open: 5}

	got := withRealDayVolume(tracker, "SPY", snapshot)

	if got.PreMarketVolume != 10 || got.PostMarketVolume != 20 || got.Open != 5 {
		t.Fatalf("got %+v, only DayVolume should change", got)
	}
}
