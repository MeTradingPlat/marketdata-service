package ingestion_test

import (
	"context"
	"testing"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/ingestion"
)

func TestGetCandles(t *testing.T) {
	repo := &fakeRepo{getResult: []domain.Candle{{Symbol: "AAPL"}}}
	svc := ingestion.NewGetCandlesService(repo)

	got, err := svc.GetCandles(context.Background(), "AAPL", domain.D1, 10, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d candles, want 1", len(got))
	}
}
