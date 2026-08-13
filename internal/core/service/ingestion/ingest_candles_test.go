package ingestion_test

import (
	"context"
	"errors"
	"testing"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/ingestion"
)

func TestBackfill(t *testing.T) {
	tests := []struct {
		name        string
		probeResult []domain.Candle
		probeErr    error
		wantSaves   int
		wantErr     bool
	}{
		{
			name:        "saves candles returned by the gateway",
			probeResult: []domain.Candle{{Symbol: "AAPL", Timeframe: domain.D1, Open: 1, High: 1, Low: 1, Close: 1}},
			wantSaves:   1,
		},
		{
			name:      "no candles from gateway does not call save",
			wantSaves: 0,
		},
		{
			name:     "gateway error propagates",
			probeErr: errors.New("boom"),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gw := &fakeGateway{probeResult: tt.probeResult, probeErr: tt.probeErr}
			repo := &fakeRepo{}
			svc := ingestion.NewIngestCandlesService(gw, repo)

			err := svc.Backfill(context.Background(), "AAPL", domain.D1)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(repo.saved) != tt.wantSaves {
				t.Fatalf("Save called %d times, want %d", len(repo.saved), tt.wantSaves)
			}
		})
	}
}

func TestStreamLive(t *testing.T) {
	gw := &fakeGateway{}
	repo := &fakeRepo{}
	svc := ingestion.NewIngestCandlesService(gw, repo)

	if err := svc.StreamLive(context.Background(), "AAPL"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gw.subscribed != "AAPL" {
		t.Fatalf("subscribed to %q, want AAPL", gw.subscribed)
	}

	gw.onCandle(domain.Candle{Symbol: "AAPL", Timeframe: domain.M1, Open: 1, High: 1, Low: 1, Close: 1})

	if len(repo.saved) != 1 {
		t.Fatalf("Save called %d times, want 1", len(repo.saved))
	}
}
