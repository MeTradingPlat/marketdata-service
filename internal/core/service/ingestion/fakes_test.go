package ingestion_test

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

type fakeGateway struct {
	probeResult    []domain.Candle
	probeErr       error
	subscribed     string
	subscribedFrom time.Time
	onCandle       func(domain.Candle)
}

func (f *fakeGateway) GetCandles(ctx context.Context, symbol string, tf domain.Timeframe, from time.Time) ([]domain.Candle, error) {
	return nil, nil
}

func (f *fakeGateway) ProbeMaxDepth(ctx context.Context, symbol string, tf domain.Timeframe) ([]domain.Candle, error) {
	return f.probeResult, f.probeErr
}

func (f *fakeGateway) SubscribeLiveCandles(ctx context.Context, symbol string, from time.Time, onCandle func(domain.Candle)) error {
	f.subscribed = symbol
	f.subscribedFrom = from
	f.onCandle = onCandle
	return nil
}

func (f *fakeGateway) ActiveSymbols(ctx context.Context) ([]domain.Symbol, error) {
	return nil, nil
}

type fakeRepo struct {
	saved     [][]domain.Candle
	saveErr   error
	getResult []domain.Candle
	watermark *time.Time
}

func (f *fakeRepo) Save(ctx context.Context, candles []domain.Candle) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, candles)
	return nil
}

func (f *fakeRepo) GetCandles(ctx context.Context, symbol string, tf domain.Timeframe, bars int) ([]domain.Candle, error) {
	return f.getResult, nil
}

func (f *fakeRepo) GetWatermark(ctx context.Context, symbol string, tf domain.Timeframe) (*time.Time, *time.Time, error) {
	return f.watermark, nil, nil
}

func (f *fakeRepo) SymbolsWithData(ctx context.Context, tf domain.Timeframe) (map[string]struct{}, error) {
	return nil, nil
}
