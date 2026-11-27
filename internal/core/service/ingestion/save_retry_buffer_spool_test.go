package ingestion

import (
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/spool"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestSaveRetryBuffer_SpoolAndFallbackDiscard(t *testing.T) {
	alerts := 0
	buf := &saveRetryBuffer{
		now:    func() time.Time { return time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC) },
		spool:  spool.NewFileSpool(t.TempDir(), 1024*1024),
		notify: func(string, string, map[string]any) { alerts++ },
	}
	buf.requeue(make([]domain.Candle, saveRetryBufferLimit))
	buf.requeue([]domain.Candle{{Symbol: "AAPL"}, {Symbol: "MSFT"}})
	if buf.discardedCount() != 0 || alerts != 0 {
		t.Fatalf("got discarded=%d alerts=%d, want 0 and 0", buf.discardedCount(), alerts)
	}
	ids, err := buf.spool.Pending()
	if err != nil || len(ids) != 1 {
		t.Fatalf("Pending = (%v, %v), want 1 batch", ids, err)
	}

	tiny := &saveRetryBuffer{
		now:    func() time.Time { return time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC) },
		spool:  spool.NewFileSpool(t.TempDir(), 10),
		notify: func(string, string, map[string]any) { alerts++ },
	}
	tiny.requeue(make([]domain.Candle, saveRetryBufferLimit))
	tiny.requeue([]domain.Candle{{Symbol: "NVDA"}})
	if tiny.discardedCount() != 1 || alerts != 1 {
		t.Fatalf("got discarded=%d alerts=%d, want 1 and 1", tiny.discardedCount(), alerts)
	}
}
