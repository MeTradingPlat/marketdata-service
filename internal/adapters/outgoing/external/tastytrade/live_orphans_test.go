package tastytrade

import (
	"context"
	"reflect"
	"testing"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestCandlePool_OrphanedLiveSymbols(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	p.allocator = newChannelAllocator(factory, nil, nil, 1)

	ch, err := p.allocator.allocate(context.Background())
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	ch.occupy(candleKey("AAPL", domain.M1))

	p.liveSubs["AAPL"] = func(domain.Candle) {}
	p.liveSubs["MSFT"] = func(domain.Candle) {}

	orphans := p.orphanedLiveSymbols()
	expected := []string{"MSFT"}
	if !reflect.DeepEqual(orphans, expected) {
		t.Fatalf("expected orphans %v, got %v", expected, orphans)
	}
}
