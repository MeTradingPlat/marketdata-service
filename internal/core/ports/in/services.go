package in

import (
	"context"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

type IngestCandlesService interface {
	Backfill(ctx context.Context, symbol string, timeframe domain.Timeframe) error
	StreamLive(ctx context.Context, symbol string) error
}

type GetCandlesService interface {
	GetCandles(ctx context.Context, symbol string, timeframe domain.Timeframe, bars int) ([]domain.Candle, error)
}
