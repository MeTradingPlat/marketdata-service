package ingestion_test

import (
	"context"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/ingestion"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/livecandles"
)

func TestGetCandles(t *testing.T) {
	repo := &fakeRepo{getResult: []domain.Candle{{Symbol: "AAPL"}}}
	svc := ingestion.NewGetCandlesService(repo, livecandles.NewDefaultRecentCache())

	got, err := svc.GetCandles(context.Background(), "AAPL", domain.D1, 10, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d candles, want 1", len(got))
	}
}

// Regression: una fila de un minuto recien cerrado podia no ser visible
// todavia en Postgres cuando alguien pedia "M1 hasta ahora" -- confirmado
// en vivo el 2026-08-27 con EMAT. GetCandles(M1, before=nil) debe traer lo
// mas reciente de RecentCache, no solo lo que la BD ya tenga.
func TestGetCandles_M1UpToNow_UsesRecentCacheForTheTail(t *testing.T) {
	older := domain.Candle{Symbol: "EMAT", Timeframe: domain.M1, Timestamp: time.Unix(1000, 0)}
	repo := &fakeRepo{getResult: []domain.Candle{older}}

	cache := livecandles.NewDefaultRecentCache()
	freshest := domain.Candle{Symbol: "EMAT", Timeframe: domain.M1, Timestamp: time.Unix(1060, 0), Volume: 27165}
	cache.Put(freshest, true)

	svc := ingestion.NewGetCandlesService(repo, cache)

	got, err := svc.GetCandles(context.Background(), "EMAT", domain.M1, 10, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d candles, want 2 (db + cache)", len(got))
	}
	if got[len(got)-1].Volume != 27165 {
		t.Errorf("expected the freshest candle from the cache, got volume %d", got[len(got)-1].Volume)
	}
}
