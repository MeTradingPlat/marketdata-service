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

// Regression: el caso real de EMAT (2026-08-27, RELATIVE_VOLUME en M5) --
// una consulta de un timeframe DERIVADO "hasta ahora" tambien debe usar el
// M1 fresco de RecentCache, no solo M1 directo. El bucket M5 se arma
// plegando el M1 cacheado, sin necesidad de un cache propio por timeframe.
func TestGetCandles_DerivedTimeframeUpToNow_FoldsFreshM1FromCache(t *testing.T) {
	bucketStart := time.Date(2026, 8, 27, 18, 10, 0, 0, time.UTC)
	staleFromDB := domain.Candle{
		Symbol: "EMAT", Timeframe: domain.M5, Timestamp: bucketStart,
		Open: 3.15, High: 3.15, Low: 3.15, Close: 3.15, Volume: 100, // "vista antes de completarse"
	}
	repo := &fakeRepo{getResult: []domain.Candle{staleFromDB}}

	cache := livecandles.NewDefaultRecentCache()
	// Las velas M1 reales del bucket 18:10-18:15, con el volumen real
	// (27165 en el minuto del pico, como el caso real).
	m1s := []domain.Candle{
		{Symbol: "EMAT", Timeframe: domain.M1, Timestamp: bucketStart, Open: 3.18, High: 3.18, Low: 3.18, Close: 3.18, Volume: 200},
		{Symbol: "EMAT", Timeframe: domain.M1, Timestamp: bucketStart.Add(time.Minute), Open: 3.18, High: 3.18, Low: 3.18, Close: 3.18, Volume: 400},
		{Symbol: "EMAT", Timeframe: domain.M1, Timestamp: bucketStart.Add(3 * time.Minute), Open: 3.18, High: 3.18, Low: 3.18, Close: 3.18, Volume: 7000},
		{Symbol: "EMAT", Timeframe: domain.M1, Timestamp: bucketStart.Add(4 * time.Minute), Open: 3.15, High: 3.15, Low: 3.06, Close: 3.06, Volume: 27165},
	}
	for _, c := range m1s {
		cache.Put(c, true)
	}

	svc := ingestion.NewGetCandlesService(repo, cache)

	got, err := svc.GetCandles(context.Background(), "EMAT", domain.M5, 10, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 folded M5 candle, got %d", len(got))
	}
	const wantVolume = 200 + 400 + 7000 + 27165
	if got[0].Volume != wantVolume {
		t.Errorf("expected the folded volume %d (from fresh M1), got %d (stale DB value was %d)", wantVolume, got[0].Volume, staleFromDB.Volume)
	}
	if got[0].Close != 3.06 {
		t.Errorf("expected close 3.06 (from fresh M1), got %v", got[0].Close)
	}
}
