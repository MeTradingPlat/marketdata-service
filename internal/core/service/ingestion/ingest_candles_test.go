package ingestion_test

import (
	"context"
	"errors"
	"testing"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/ingestion"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
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
			svc := ingestion.NewIngestCandlesService(gw, repo, livecandles.NewBroadcaster())

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
	svc := ingestion.NewIngestCandlesService(gw, repo, livecandles.NewBroadcaster())

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

func TestStreamLive_BuffersFailedSaveForRetry(t *testing.T) {
	gw := &fakeGateway{}
	repo := &fakeRepo{saveErr: errors.New("db down")}
	svc := ingestion.NewIngestCandlesService(gw, repo, livecandles.NewBroadcaster())

	if err := svc.StreamLive(context.Background(), "AAPL"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gw.onCandle(domain.Candle{Symbol: "AAPL", Timeframe: domain.M1, Open: 1, High: 1, Low: 1, Close: 1})
	if len(repo.saved) != 0 {
		t.Fatalf("Save should have failed and not recorded a save, got %d", len(repo.saved))
	}

	repo.saveErr = nil
	svc.RetryPendingSaves(context.Background())

	if len(repo.saved) != 1 {
		t.Fatalf("expected the buffered candle to be saved on retry, got %d saves", len(repo.saved))
	}
	if len(repo.saved[0]) != 1 || repo.saved[0][0].Symbol != "AAPL" {
		t.Fatalf("unexpected saved candle: %+v", repo.saved)
	}

	repo.saved = nil
	svc.RetryPendingSaves(context.Background())
	if len(repo.saved) != 0 {
		t.Fatalf("expected empty buffer to result in no further Save call, got %d", len(repo.saved))
	}
}
