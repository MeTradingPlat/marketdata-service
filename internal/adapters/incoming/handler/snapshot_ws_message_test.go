package handler

import (
	"testing"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
)

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

func TestFundamentalsMessage_WrapsSymbolAndFundamentals(t *testing.T) {
	f := domain.Fundamentals{Symbol: "AAPL", MarketCap: 3_000_000}

	got := fundamentalsMessage("AAPL", f)

	msg, ok := got.(dto.FundamentalsMessage)
	if !ok {
		t.Fatalf("fundamentalsMessage returned %T, want dto.FundamentalsMessage", got)
	}
	if msg.Type != "fundamentals" || msg.Symbol != "AAPL" || msg.Fundamentals.MarketCap != 3_000_000 {
		t.Fatalf("unexpected message: %+v", msg)
	}
}
