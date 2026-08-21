package livecandles

import (
	"context"
	"testing"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

type fakeCandlesService struct {
	bars []domain.Candle
}

func (f *fakeCandlesService) GetCandles(ctx context.Context, symbol string, timeframe domain.Timeframe, bars int, before *time.Time) ([]domain.Candle, error) {
	return f.bars, nil
}

func (f *fakeCandlesService) GetCandlesBatch(ctx context.Context, symbols []string, timeframe domain.Timeframe, bars int) map[string][]domain.Candle {
	return nil
}

type fakeGateway struct {
	current   domain.Candle
	currentOK bool
}

func (f *fakeGateway) GetCandles(ctx context.Context, symbol string, timeframe domain.Timeframe, from time.Time) ([]domain.Candle, error) {
	return nil, nil
}
func (f *fakeGateway) GetCandlesBatch(ctx context.Context, timeframe domain.Timeframe, froms map[string]time.Time) (map[string][]domain.Candle, error) {
	return nil, nil
}
func (f *fakeGateway) ProbeMaxDepth(ctx context.Context, symbol string, timeframe domain.Timeframe) ([]domain.Candle, error) {
	return nil, nil
}
func (f *fakeGateway) SubscribeLiveCandles(ctx context.Context, symbol string, from time.Time, onClosed func(domain.Candle), onTick func(domain.Candle)) error {
	return nil
}
func (f *fakeGateway) ActiveSymbols(ctx context.Context) ([]domain.Symbol, error) { return nil, nil }
func (f *fakeGateway) CurrentCandle(symbol string) (domain.Candle, bool)          { return f.current, f.currentOK }
func (f *fakeGateway) LiveSubscribed(symbol string) bool                         { return false }
func (f *fakeGateway) DividendInfo(ctx context.Context, symbols []string) ([]domain.Fundamentals, error) {
	return nil, nil
}
func (f *fakeGateway) MarketMetrics(ctx context.Context, symbols []string) ([]domain.Fundamentals, error) {
	return nil, nil
}
func (f *fakeGateway) EarningsReports(ctx context.Context, symbol string) ([]domain.EarningsReportItem, error) {
	return nil, nil
}
func (f *fakeGateway) ResetLiveConnections() {}

func TestGetCurrentCandle_M1_NoLiveTick_ReturnsNilInsteadOfFlatBar(t *testing.T) {
	// Regression: antes fabricaba una vela plana (open=high=low=close =
	// ultimo cierre) cuando el minuto no tuvo ningun trade -- confirmado en
	// vivo un candle fantasma dibujado en post-mercado para un minuto sin
	// ticks reales.
	svc := NewCurrentCandleService(&fakeCandlesService{bars: []domain.Candle{{Close: 10}}}, &fakeGateway{currentOK: false})

	bar, err := svc.GetCurrentCandle(context.Background(), "AAPL", domain.M1)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bar != nil {
		t.Fatalf("expected nil forming bar with no live tick, got %+v", bar)
	}
}

func TestGetCurrentCandle_M1_WithLiveTick_ReturnsRealBar(t *testing.T) {
	now := time.Now().UTC()
	period := FormingPeriodStart(now, domain.M1)
	live := domain.Candle{Timestamp: period, Open: 1, High: 2, Low: 0.5, Close: 1.5}
	svc := NewCurrentCandleService(&fakeCandlesService{}, &fakeGateway{current: live, currentOK: true})

	bar, err := svc.GetCurrentCandle(context.Background(), "AAPL", domain.M1)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bar == nil || bar.Close != 1.5 {
		t.Fatalf("expected the real live bar, got %+v", bar)
	}
}

func TestGetCurrentCandle_DerivedTimeframe_NoRealData_ReturnsNil(t *testing.T) {
	svc := NewCurrentCandleService(&fakeCandlesService{bars: nil}, &fakeGateway{currentOK: false})

	bar, err := svc.GetCurrentCandle(context.Background(), "AAPL", domain.M5)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bar != nil {
		t.Fatalf("expected nil forming bar with no real M1 data in the period, got %+v", bar)
	}
}

func TestGetCurrentCandle_DerivedTimeframe_PartialRealM1Data_ReturnsRealBar(t *testing.T) {
	now := time.Now().UTC()
	period := FormingPeriodStart(now, domain.M5)
	m1 := domain.Candle{Timestamp: period, Open: 3, High: 4, Low: 2, Close: 3.5}
	svc := NewCurrentCandleService(&fakeCandlesService{bars: []domain.Candle{m1}}, &fakeGateway{currentOK: false})

	bar, err := svc.GetCurrentCandle(context.Background(), "AAPL", domain.M5)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bar == nil || bar.Open != 3 || bar.Close != 3.5 {
		t.Fatalf("expected a bar built from the real M1 data, got %+v", bar)
	}
}
