package handler

import (
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
)

func TestAggregate_EveryDerivedTimeframeClosesWithTheSumOfItsMinutes(t *testing.T) {
	previous := aggregateCloseDelay
	aggregateCloseDelay = 24 * time.Hour
	defer func() { aggregateCloseDelay = previous }()
	cases := []struct {
		tf      domain.Timeframe
		minutes int
	}{
		{domain.M2, 2}, {domain.M3, 3}, {domain.M5, 5}, {domain.M10, 10}, {domain.M15, 15},
		{domain.M30, 30}, {domain.H1, 60}, {domain.H2, 120}, {domain.H4, 240},
	}
	for _, tc := range cases {
		t.Run(string(tc.tf), func(t *testing.T) {
			raw := livecandles.NewBroadcaster[domain.Candle]()
			hub := newCandleAggregateHub(raw, nilCurrentCandleService{})
			ch, cancel := subscribeToChanSized(hub, "AAPL", string(tc.tf), tc.tf, tc.minutes*2+8)
			defer cancel()
			start := livecandles.FormingPeriodStart(time.Now().UTC(), tc.tf)

			var want int64
			for i := 0; i < tc.minutes; i++ {
				minute := start.Add(time.Duration(i) * time.Minute)
				raw.Publish("AAPL", m1(minute, int64(10*(i+1))))
				raw.Publish("AAPL", m1(minute, int64(100*(i+1))))
				want += int64(100 * (i + 1))
			}
			raw.Publish("AAPL", m1(livecandles.NextPeriodStart(start, tc.tf), 5))

			var closed *closedBar
			for len(ch) > 0 {
				bar := <-ch
				if bar.Closed && bar.Time == start.Unix() {
					closed = &closedBar{volume: bar.Volume}
				}
			}
			if closed == nil {
				t.Fatalf("%s: the period never closed", tc.tf)
			}
			if closed.volume != want {
				t.Fatalf("%s: closed volume = %d, want %d (sum of the final volume of each minute)", tc.tf, closed.volume, want)
			}
		})
	}
}

type closedBar struct{ volume int64 }
