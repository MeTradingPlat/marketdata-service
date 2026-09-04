package intraday

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// fakeSlowRepo simula el fallback a BD: cada GetCandles tarda un poco y
// registra cuantas llamadas estan en vuelo al mismo tiempo -- para probar
// que dbFallbackConcurrencyLimit de verdad topa el paralelismo GLOBAL, no
// solo el de un request individual.
type fakeSlowRepo struct {
	inFlight  atomic.Int32
	maxSeen   atomic.Int32
	callDelay time.Duration
}

func (f *fakeSlowRepo) GetCandles(ctx context.Context, symbol string, tf domain.Timeframe, bars int, before *time.Time) ([]domain.Candle, error) {
	n := f.inFlight.Add(1)
	defer f.inFlight.Add(-1)
	for {
		max := f.maxSeen.Load()
		if n <= max || f.maxSeen.CompareAndSwap(max, n) {
			break
		}
	}
	time.Sleep(f.callDelay)
	return []domain.Candle{{Close: 1.23}}, nil
}

func (f *fakeSlowRepo) Save(ctx context.Context, candles []domain.Candle, withWatermark bool) error {
	return nil
}
func (f *fakeSlowRepo) GetSeries(ctx context.Context, symbols []string, tf domain.Timeframe, bars int) (map[string][]domain.Candle, error) {
	return nil, nil
}
func (f *fakeSlowRepo) GetSeriesPriority(ctx context.Context, symbols []string, tf domain.Timeframe, bars int) (map[string][]domain.Candle, error) {
	return nil, nil
}
func (f *fakeSlowRepo) GetSeriesAggregatedBatch(ctx context.Context, symbols []string, timeframe, source domain.Timeframe, bucket string, approxPeriod time.Duration, bars int) (map[string][]domain.Candle, error) {
	return nil, nil
}
func (f *fakeSlowRepo) GetWatermark(ctx context.Context, symbol string, tf domain.Timeframe) (*time.Time, error) {
	return nil, nil
}
func (f *fakeSlowRepo) GetWatermarksBatch(ctx context.Context, symbols []string, tf domain.Timeframe) (map[string]time.Time, error) {
	return nil, nil
}
func (f *fakeSlowRepo) SymbolsWithData(ctx context.Context, tf domain.Timeframe) (map[string]struct{}, error) {
	return nil, nil
}
func (f *fakeSlowRepo) GetIntradaySessions(ctx context.Context, symbol string) (domain.IntradaySnapshot, error) {
	return domain.IntradaySnapshot{}, nil
}
func (f *fakeSlowRepo) GetIntradaySessionsBatch(ctx context.Context, symbols []string) (map[string]domain.IntradaySnapshot, error) {
	return nil, nil
}
func (f *fakeSlowRepo) GetPreviousSessionClose(ctx context.Context, symbol string, before time.Time) (*float64, error) {
	return nil, nil
}
func (f *fakeSlowRepo) GetPreviousSessionCloseBatch(ctx context.Context, symbols []string, before time.Time) (map[string]float64, error) {
	return nil, nil
}
func (f *fakeSlowRepo) GetPreviousPostMarketVolumeBatch(ctx context.Context, symbols []string, before time.Time) (map[string]int64, error) {
	return nil, nil
}

// noLiveGateway: siempre falla CurrentCandle -- fuerza que resolvePrice
// caiga al fallback de BD para todos los simbolos.
type noLiveGateway struct{}

func (noLiveGateway) GetCandles(ctx context.Context, symbol string, tf domain.Timeframe, from time.Time) ([]domain.Candle, error) {
	return nil, nil
}
func (noLiveGateway) GetCandlesBatch(ctx context.Context, tf domain.Timeframe, froms map[string]time.Time) (map[string][]domain.Candle, error) {
	return nil, nil
}
func (noLiveGateway) ProbeMaxDepth(ctx context.Context, symbol string, tf domain.Timeframe) ([]domain.Candle, error) {
	return nil, nil
}
func (noLiveGateway) SubscribeLiveCandles(ctx context.Context, symbol string, from time.Time, onClosed func(domain.Candle), onTick func(domain.Candle)) error {
	return nil
}
func (noLiveGateway) ActiveSymbols(ctx context.Context) ([]domain.Symbol, error) { return nil, nil }
func (noLiveGateway) CurrentCandle(symbol string) (domain.Candle, bool)          { return domain.Candle{}, false }
func (noLiveGateway) LiveSubscribed(symbol string) bool                         { return false }
func (noLiveGateway) DividendInfo(ctx context.Context, symbols []string) ([]domain.Fundamentals, error) {
	return nil, nil
}
func (noLiveGateway) MarketMetrics(ctx context.Context, symbols []string) ([]domain.Fundamentals, error) {
	return nil, nil
}
func (noLiveGateway) EarningsReports(ctx context.Context, symbol string) ([]domain.EarningsReportItem, error) {
	return nil, nil
}
func (noLiveGateway) ResetLiveConnections() {}

func TestGetCurrentPrices_DBFallbackNeverExceedsGlobalLimit(t *testing.T) {
	repo := &fakeSlowRepo{callDelay: 20 * time.Millisecond}
	svc := NewGetCurrentPricesService(repo, noLiveGateway{}, NewSnapshotTracker())

	symbols := make([]string, 60)
	for i := range symbols {
		symbols[i] = "SYM" + string(rune('A'+i%26)) + string(rune('0'+i/26))
	}

	// Simula 3 "escaneres" llamando GetCurrentPrices al mismo tiempo, cada
	// uno con su propio lote -- el limite debe seguir siendo GLOBAL, no por
	// llamada.
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.GetCurrentPrices(context.Background(), symbols)
		}()
	}
	wg.Wait()

	if max := repo.maxSeen.Load(); max > dbFallbackConcurrencyLimit {
		t.Fatalf("max concurrent DB calls = %d, want <= %d", max, dbFallbackConcurrencyLimit)
	}
}

func TestGetCurrentPrices_LiveAndTrackerHitsSkipDBEntirely(t *testing.T) {
	repo := &fakeSlowRepo{callDelay: time.Millisecond}
	tracker := NewSnapshotTracker()
	tracker.SeedLastClose(map[string]domain.Candle{"AAPL": {Close: 42}})
	svc := NewGetCurrentPricesService(repo, noLiveGateway{}, tracker)

	prices := svc.GetCurrentPrices(context.Background(), []string{"AAPL"})

	if prices["AAPL"] != 42 {
		t.Fatalf("expected tracker price 42, got %+v", prices)
	}
	if repo.maxSeen.Load() != 0 {
		t.Fatal("expected the DB fallback to never be called when the tracker already has the price")
	}
}
