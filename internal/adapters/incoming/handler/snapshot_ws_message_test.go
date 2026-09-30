package handler

import (
	"context"
	"testing"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
)

type fakeFundamentalsRealtimeService func(ctx context.Context, symbols []string) map[string]dto.FundamentalRealtime

func (f fakeFundamentalsRealtimeService) GetFundamentalsRealtime(ctx context.Context, symbols []string) map[string]dto.FundamentalRealtime {
	return f(ctx, symbols)
}

func TestSnapshotMessage_WrapsSymbolAndSnapshot(t *testing.T) {
	snap := domain.IntradaySnapshot{Symbol: "AAPL", CurrentPrice: 150, DayVolume: 1000}

	got := snapshotMessage("AAPL", snap)

	msg, ok := got.(dto.SnapshotMessage)
	if !ok {
		t.Fatalf("snapshotMessage returned %T, want dto.SnapshotMessage", got)
	}
	if msg.Type != "snapshot" || msg.Symbol != "AAPL" || msg.Snapshot.CurrentPrice != 150 {
		t.Fatalf("unexpected message: %+v", msg)
	}
}

func TestFundamentalsMessage_WrapsSymbolAndTheEnrichedRealtimeDTO(t *testing.T) {
	marketCap := 3_000_000.0
	svc := fakeFundamentalsRealtimeService(func(_ context.Context, symbols []string) map[string]dto.FundamentalRealtime {
		return map[string]dto.FundamentalRealtime{symbols[0]: {Symbol: symbols[0], MarketCap: &marketCap}}
	})
	h := &FundamentalsWSHandler{realtimeService: svc}

	got := h.fundamentalsMessage("AAPL", domain.Fundamentals{Symbol: "AAPL"})

	msg, ok := got.(dto.FundamentalsMessage)
	if !ok {
		t.Fatalf("fundamentalsMessage returned %T, want dto.FundamentalsMessage", got)
	}
	if msg.Type != "fundamentals" || msg.Symbol != "AAPL" || msg.Fundamentals.MarketCap == nil || *msg.Fundamentals.MarketCap != marketCap {
		t.Fatalf("unexpected message: %+v", msg)
	}
}
