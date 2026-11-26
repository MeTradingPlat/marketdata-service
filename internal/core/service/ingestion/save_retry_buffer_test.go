package ingestion

import (
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestSaveRetryBuffer_OverflowAndAlertThrottle(t *testing.T) {
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	alerts := 0
	buf := &saveRetryBuffer{
		now: func() time.Time { return now },
		notify: func(level, _ string, meta map[string]any) {
			alerts++
			if level != "ERROR" || meta["totalDescartadas"] == nil {
				t.Fatalf("unexpected alert payload: level=%s meta=%v", level, meta)
			}
		},
	}

	fill := make([]domain.Candle, saveRetryBufferLimit)
	buf.requeue(fill)
	if buf.size() != saveRetryBufferLimit || buf.discardedCount() != 0 || alerts != 0 {
		t.Fatalf("unexpected state at limit: size=%d discarded=%d alerts=%d", buf.size(), buf.discardedCount(), alerts)
	}

	buf.requeue([]domain.Candle{{Symbol: "AAPL"}, {Symbol: "MSFT"}, {Symbol: "AAPL"}})
	if buf.discardedCount() != 3 || alerts != 1 {
		t.Fatalf("got discarded=%d alerts=%d, want 3 and 1", buf.discardedCount(), alerts)
	}

	now = now.Add(5 * time.Minute)
	buf.add(domain.Candle{Symbol: "NVDA"})
	if buf.discardedCount() != 4 || alerts != 1 {
		t.Fatalf("alert fired before 15m throttle: discarded=%d alerts=%d", buf.discardedCount(), alerts)
	}

	now = now.Add(11 * time.Minute)
	buf.add(domain.Candle{Symbol: "TSLA"})
	if buf.discardedCount() != 5 || alerts != 2 {
		t.Fatalf("expected second alert after 15m: discarded=%d alerts=%d", buf.discardedCount(), alerts)
	}
}
