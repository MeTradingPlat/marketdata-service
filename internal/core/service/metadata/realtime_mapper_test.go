package metadata

import (
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestToRealtime_SectionsPresenceAndAbsence(t *testing.T) {
	now := time.Now()
	shares, flt, shortShares := int64(1000), int64(500), int64(50)
	shortLegacy, shortRatio, oi := 12.5, 2.5, 900.0
	prevPM := int64(300)

	empty := toRealtime("AAPL", domain.Symbol{}, domain.Fundamentals{}, domain.IntradaySnapshot{})
	if empty.IsEtf != nil || empty.SecurityType != nil || empty.DividendAmount != nil ||
		empty.MarketCap != nil || empty.SharesOutstanding != nil || empty.PrevPostMarketVolume != nil || empty.OpenInterest != nil {
		t.Fatalf("expected all optional sections nil, got %+v", empty)
	}

	fullF := domain.Fundamentals{
		MarketDataUpdatedAt: &now, DividendAmount: 0.25, DividendFrequency: 4,
		TradingStatus: "ACTIVE", HaltStartTime: 100, HaltEndTime: -1,
		MetricsUpdatedAt: &now, Beta: 1.1,
		ExternalUpdatedAt: &now, SharesOutstanding: &shares, FloatShares: &flt,
		ShortInterestShares: &shortShares, ShortRatio: &shortRatio,
		PrevPostMarketVolumeUpdatedAt: &now, PrevPostMarketVolume: &prevPM, OpenInterest: &oi,
	}
	full := toRealtime("AAPL", domain.Symbol{Symbol: "AAPL", IsEtf: true}, fullF, domain.IntradaySnapshot{})
	if full.IsEtf == nil || !*full.IsEtf || full.SecurityType == nil || full.DividendAmount == nil ||
		full.HaltStartTime == nil || full.HaltEndTime != nil || full.Beta == nil ||
		full.ShortInterest == nil || *full.ShortInterest != 10 || full.PrevPostMarketVolume == nil || full.OpenInterest == nil {
		t.Fatalf("unexpected full realtime: %+v", full)
	}

	fallbackF := domain.Fundamentals{ExternalUpdatedAt: &now, ShortInterest: &shortLegacy}
	fb := toRealtime("AAPL", domain.Symbol{}, fallbackF, domain.IntradaySnapshot{})
	if fb.ShortInterest == nil || *fb.ShortInterest != 12.5 {
		t.Fatalf("ShortInterest fallback = %v, want 12.5", fb.ShortInterest)
	}
}
