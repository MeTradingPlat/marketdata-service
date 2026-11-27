package tastytrade

import (
	"context"
	"testing"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestBatchCleanup_SkipsLiveOnSameChannel(t *testing.T) {
	wsURL, rec := startSubFeedRecordingServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.allocator = newChannelAllocator(factory, p.wireChannel, p.handleConnectionReconnect, 2)

	ch, err := p.allocator.allocate(context.Background(), candleKey("AAPL", domain.M1), candleKey("MSFT", domain.M1))
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	p.registerLiveSub("AAPL", func(domain.Candle) {}, nil, false, 0)

	p.batchCleanup(ch, []string{"AAPL", "MSFT"}, domain.M1, map[string]uint64{"AAPL": 0, "MSFT": 0})()
	rec.waitCount(t, 2)
	for _, s := range rec.snapshot() {
		if s.Action == "remove" && s.Symbol == "AAPL{=1m}" {
			t.Fatalf("unexpected remove for live symbol AAPL: %+v", rec.snapshot())
		}
	}
	if !channelHasKey(ch, candleKey("AAPL", domain.M1)) || channelHasKey(ch, candleKey("MSFT", domain.M1)) {
		t.Fatal("expected AAPL kept and MSFT released")
	}
}

func TestBatchCleanup_RemovesLiveOnOtherChannelAndNonM1(t *testing.T) {
	wsURL, rec := startSubFeedRecordingServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.allocator = newChannelAllocator(factory, p.wireChannel, p.handleConnectionReconnect, 2)
	p.SetMaxChannelsPerConn(2)

	ch1, _ := p.allocator.allocate(context.Background(), candleKey("AAPL", domain.M1))
	ch2, _ := p.allocator.allocateExcluding(context.Background(), ch1, candleKey("AAPL", domain.M1), candleKey("AAPL", domain.D1))
	p.registerLiveSub("AAPL", func(domain.Candle) {}, nil, false, 0)

	p.batchCleanup(ch2, []string{"AAPL"}, domain.M1, map[string]uint64{"AAPL": 0})()
	rec.waitCount(t, 2)
	if channelHasKey(ch2, candleKey("AAPL", domain.M1)) || !channelHasKey(ch1, candleKey("AAPL", domain.M1)) {
		t.Fatal("expected ch2 released and ch1 intact")
	}
	p.batchCleanup(ch2, []string{"AAPL"}, domain.D1, map[string]uint64{"AAPL": 0})()
	rec.waitCount(t, 2)
	if channelHasKey(ch2, candleKey("AAPL", domain.D1)) {
		t.Fatal("expected D1 key released even when AAPL is live")
	}
}
