package intraday

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// fakeSlowRepo simula el fallback a BD: registra cuantas veces se llamo
// GetSeriesPriority y con cuantos simbolos -- el fallback debe ser SIEMPRE
// un solo lote, nunca un GetCandles por simbolo (ver el comentario de
// GetCurrentPrices sobre el incidente del 2026-09-04).
type fakeSlowRepo struct {
	callCount    atomic.Int32
	lastBatchLen atomic.Int32
	callDelay    time.Duration
}

func (f *fakeSlowRepo) GetCandles(ctx context.Context, symbol string, tf domain.Timeframe, bars int, before *time.Time) ([]domain.Candle, error) {
	return nil, nil
}

func (f *fakeSlowRepo) Save(ctx context.Context, candles []domain.Candle, withWatermark bool) error {
	return nil
}
func (f *fakeSlowRepo) GetSeries(ctx context.Context, symbols []string, tf domain.Timeframe, bars int) (map[string][]domain.Candle, error) {
	return nil, nil
}
func (f *fakeSlowRepo) GetSeriesPriority(ctx context.Context, symbols []string, tf domain.Timeframe, bars int) (map[string][]domain.Candle, error) {
	f.callCount.Add(1)
	f.lastBatchLen.Store(int32(len(symbols)))
	time.Sleep(f.callDelay)
	result := make(map[string][]domain.Candle, len(symbols))
	for _, sym := range symbols {
		result[sym] = []domain.Candle{{Close: 1.23}}
	}
	return result, nil
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

func TestGetCurrentPrices_DBFallbackIsOneBatchCallNotOnePerSymbol(t *testing.T) {
	repo := &fakeSlowRepo{callDelay: time.Millisecond}
	svc := NewGetCurrentPricesService(repo, noLiveGateway{}, NewSnapshotTracker())

	symbols := make([]string, 60)
	for i := range symbols {
		symbols[i] = "SYM" + string(rune('A'+i%26)) + string(rune('0'+i/26))
	}

	prices := svc.GetCurrentPrices(context.Background(), symbols)

	if got := repo.callCount.Load(); got != 1 {
		t.Fatalf("GetSeriesPriority calls = %d, want exactly 1 (one batch, not one per symbol)", got)
	}
	if got := repo.lastBatchLen.Load(); int(got) != len(symbols) {
		t.Fatalf("batch size = %d, want %d (all symbols in one call)", got, len(symbols))
	}
	if len(prices) != len(symbols) {
		t.Fatalf("resolved %d prices, want %d", len(prices), len(symbols))
	}
}

func TestGetCurrentPrices_ConcurrentCallersEachBatchOnce(t *testing.T) {
	repo := &fakeSlowRepo{callDelay: 5 * time.Millisecond}
	svc := NewGetCurrentPricesService(repo, noLiveGateway{}, NewSnapshotTracker())

	symbols := []string{"AAPL", "MSFT", "NVDA"}

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.GetCurrentPrices(context.Background(), symbols)
		}()
	}
	wg.Wait()

	if got := repo.callCount.Load(); got != 3 {
		t.Fatalf("GetSeriesPriority calls = %d, want exactly 3 (one batch per caller)", got)
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
	if repo.callCount.Load() != 0 {
		t.Fatal("expected the DB fallback to never be called when the tracker already has the price")
	}
}
