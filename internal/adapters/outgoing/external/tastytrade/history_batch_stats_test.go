package tastytrade

import (
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestComputeHistoryBatchStats(t *testing.T) {
	requested := []string{"AAPL", "MSFT", "TSLA", "NVDA"}
	candles := map[string][]domain.Candle{
		"AAPL": {{Symbol: "AAPL"}},
		"MSFT": {{Symbol: "MSFT"}},
	}
	signaled := map[string]bool{"AAPL": true, "TSLA": true}
	snipped := map[string]bool{"AAPL": true}

	got := computeHistoryBatchStats(requested, candles, signaled, snipped)
	want := historyBatchStats{
		requested: 4, withData: 2, signaled: 2, signaledWithData: 1, snipped: 1, withoutData: 2,
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestBatchHistoryCollector_TracksSignaledAndSnipped(t *testing.T) {
	c := newBatchHistoryCollector(domain.D1)
	now := time.Now()
	c.onCandle("AAPL", completeCandleEvent(now, eventFlagSnapshotSnip))
	c.onCandle("TSLA", rawCandleEvent{Timestamp: now, EventFlags: eventFlagSnapshotEnd})
	c.onCandle("MSFT", completeCandleEvent(now, 0))

	got := c.stats([]string{"AAPL", "TSLA", "MSFT", "NVDA"}, c.complete())
	want := historyBatchStats{
		requested: 4, withData: 2, signaled: 2, signaledWithData: 1, snipped: 1, withoutData: 2,
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestChannelLogIDsAndLog(t *testing.T) {
	if c, ch := channelLogIDs(nil); c != 0 || ch != 0 {
		t.Fatalf("nil channel = (%d, %d)", c, ch)
	}
	pc := newPooledChannel(&dxLinkChannel{id: 3, client: &DxLinkConn{id: 7}})
	if c, ch := channelLogIDs(pc); c != 7 || ch != 3 {
		t.Fatalf("got (%d, %d), want (7, 3)", c, ch)
	}
	logHistoryBatchFinished(pc, domain.M1, historyBatchStats{requested: 1}, false)
}
