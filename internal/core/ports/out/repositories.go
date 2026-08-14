package out

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

type CandleRepository interface {
	Save(ctx context.Context, candles []domain.Candle) error
	GetCandles(ctx context.Context, symbol string, timeframe domain.Timeframe, bars int) ([]domain.Candle, error)
	GetWatermark(ctx context.Context, symbol string, timeframe domain.Timeframe) (newest, oldest *time.Time, err error)
	SymbolsWithData(ctx context.Context, timeframe domain.Timeframe) (map[string]struct{}, error)
}

type SymbolRepository interface {
	Upsert(ctx context.Context, symbols []domain.Symbol) error
	Tracked(ctx context.Context) ([]domain.Symbol, error)
	Deactivate(ctx context.Context, symbols []string) error
}
