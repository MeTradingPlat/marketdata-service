package tastytrade

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func TestBatchTargetSymbols_AndEmptyFetchHistoryBatch(t *testing.T) {
	p := newTestPool()
	p.liveSubs["NVDA"] = func(domain.Candle) {}
	froms := map[string]time.Time{
		"ZZZ":  time.Now(),
		"NVDA": time.Now(),
		"AAA":  time.Now(),
	}
	got := p.batchTargetSymbols(domain.M1, froms)
	want := []string{"AAA", "ZZZ"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("batchTargetSymbols = %v, want %v", got, want)
	}
	res, err := p.FetchHistoryBatch(context.Background(), domain.M1, map[string]time.Time{})
	if err != nil || len(res) != 0 {
		t.Fatalf("empty FetchHistoryBatch = (%v, %v), want (empty, nil)", res, err)
	}
}

func TestFetchHistoryBatch_CancelledContext(t *testing.T) {
	wsURL := startFakeDxLinkServer(t)
	factory, _ := newTestConnFactory(t, wsURL)
	p := newTestPool()
	p.dispatch = make(map[string]dispatchEntry)
	p.allocator = newChannelAllocator(factory, p.wireChannel, p.handleConnectionReconnect, 1)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := p.FetchHistoryBatch(ctx, domain.D1, map[string]time.Time{"AAPL": time.Now().Add(-time.Hour)})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
