package main

import (
	"context"
	"testing"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/intraday"
)

type fakeExtendedDayVolumeGateway func(symbols []string) map[string]int64

func (fakeExtendedDayVolumeGateway) FetchDayVolumes(context.Context, []string) map[string]int64 {
	return nil
}

func (f fakeExtendedDayVolumeGateway) LiveExtendedDayVolumes(symbols []string) map[string]int64 {
	return f(symbols)
}

func TestRefreshExtendedDayVolumes_UpdatesTheTrackerWithoutTouchingTheDB(t *testing.T) {
	symbols := &fakeTrackedSymbols{tracked: []domain.Symbol{{Symbol: "AAPL"}}}
	tracker := intraday.NewDayVolumeTracker()
	gateway := fakeExtendedDayVolumeGateway(func(syms []string) map[string]int64 {
		return map[string]int64{"AAPL": 512_000}
	})

	refreshExtendedDayVolumes(context.Background(), gateway, symbols, tracker)

	got, ok := tracker.Extended("AAPL")
	if !ok || got != 512_000 {
		t.Fatalf("Extended AAPL = %d ok=%v, want 512000 true", got, ok)
	}
}
