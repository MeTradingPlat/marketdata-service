package tastytrade

import (
	"reflect"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestLogLiveHealth_AndSampleUpTo10(t *testing.T) {
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.allocator = newChannelAllocator(nil, p.wireChannel, p.handleConnectionReconnect, 1)
	now := time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC)

	p.liveSubs["NVDA"] = func(domain.Candle) {}
	p.liveSubs["AAPL"] = func(domain.Candle) {}
	p.noteNewBar("NVDA", now.Add(-6*time.Minute), now.Add(-6*time.Minute))
	p.noteTrade("NVDA", now.Add(-1*time.Minute))
	p.LogLiveHealth(now)

	items := []string{"01", "02", "03", "04", "05", "06", "07", "08", "09", "10", "11"}
	if got := sampleUpTo10(items); len(got) != 10 {
		t.Fatalf("len(sampleUpTo10) = %d, want 10", len(got))
	}
	short := []string{"A", "B"}
	if got := sampleUpTo10(short); !reflect.DeepEqual(got, short) {
		t.Fatalf("sampleUpTo10(short) = %v, want %v", got, short)
	}
}
