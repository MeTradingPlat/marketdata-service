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
	onTickCandle   func(domain.Candle)
}

func (f *fakeGateway) GetCandles(ctx context.Context, symbol string, tf domain.Timeframe, from time.Time) ([]domain.Candle, error) {
	return nil, nil
}

func (f *fakeGateway) ProbeMaxDepth(ctx context.Context, symbol string, tf domain.Timeframe) ([]domain.Candle, error) {
	return f.probeResult, f.probeErr
}

func (f *fakeGateway) SubscribeLiveCandles(ctx context.Context, symbol string, from time.Time, onClosed func(domain.Candle), onTick func(domain.Candle)) error {
	f.subscribed = symbol
	f.subscribedFrom = from
	f.onCandle = onClosed
	f.onTickCandle = onTick
	return nil
}

func (f *fakeGateway) ActiveSymbols(ctx context.Context) ([]domain.Symbol, error) {
	return nil, nil
}

func (f *fakeGateway) CurrentCandle(symbol string) (domain.Candle, bool) {
	return domain.Candle{}, false
}

func (f *fakeGateway) LiveSubscribed(symbol string) bool {
	return false
}

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

type fakeRepo struct {
	saved     [][]domain.Candle
	saveErr   error
	getResult []domain.Candle
	watermark *time.Time
}

func (f *fakeRepo) Save(ctx context.Context, candles []domain.Candle, withWatermark bool) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, candles)
	return nil
}

func (f *fakeRepo) GetSeries(ctx context.Context, symbols []string, tf domain.Timeframe, bars int) (map[string][]domain.Candle, error) {
	return map[string][]domain.Candle{}, nil
}

func (f *fakeRepo) GetSeriesPriority(ctx context.Context, symbols []string, tf domain.Timeframe, bars int) (map[string][]domain.Candle, error) {
	return map[string][]domain.Candle{}, nil
}

func (f *fakeRepo) GetSeriesAggregatedBatch(ctx context.Context, symbols []string, timeframe, source domain.Timeframe, bucket string, approxPeriod time.Duration, bars int) (map[string][]domain.Candle, error) {
	return map[string][]domain.Candle{}, nil
}

func (f *fakeRepo) GetCandles(ctx context.Context, symbol string, tf domain.Timeframe, bars int, before *time.Time) ([]domain.Candle, error) {
	return f.getResult, nil
}

func (f *fakeRepo) GetWatermark(ctx context.Context, symbol string, tf domain.Timeframe) (*time.Time, error) {
	return f.watermark, nil
}

func (f *fakeRepo) GetWatermarksBatch(ctx context.Context, symbols []string, tf domain.Timeframe) (map[string]time.Time, error) {
	if f.watermark != nil {
		result := make(map[string]time.Time, len(symbols))
		for _, s := range symbols {
			result[s] = *f.watermark
		}
		return result, nil
	}
	return map[string]time.Time{}, nil
}

func (f *fakeRepo) SymbolsWithData(ctx context.Context, tf domain.Timeframe) (map[string]struct{}, error) {
	return nil, nil
}

func (f *fakeRepo) GetIntradaySessions(ctx context.Context, symbol string) (domain.IntradaySnapshot, error) {
	return domain.IntradaySnapshot{}, nil
}

func (f *fakeRepo) GetIntradaySessionsBatch(ctx context.Context, symbols []string) (map[string]domain.IntradaySnapshot, error) {
	return map[string]domain.IntradaySnapshot{}, nil
}

func (f *fakeRepo) GetPreviousSessionClose(ctx context.Context, symbol string, before time.Time) (*float64, error) {
	return nil, nil
}

func (f *fakeRepo) GetPreviousSessionCloseBatch(ctx context.Context, symbols []string, before time.Time) (map[string]float64, error) {
	return map[string]float64{}, nil
}

func (f *fakeGateway) GetCandlesBatch(ctx context.Context, timeframe domain.Timeframe, froms map[string]time.Time) (map[string][]domain.Candle, error) {
	return nil, nil
}
