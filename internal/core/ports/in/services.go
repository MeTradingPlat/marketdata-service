package in

import (
	"context"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

type IngestCandlesService interface {
	Backfill(ctx context.Context, symbol string, timeframe domain.Timeframe) error
	StreamLive(ctx context.Context, symbol string) error
	RetryPendingSaves(ctx context.Context)
}

type GetCandlesService interface {
	GetCandles(ctx context.Context, symbol string, timeframe domain.Timeframe, bars int) ([]domain.Candle, error)
}

type GetIntradaySnapshotService interface {
	GetSnapshot(ctx context.Context, symbol string) (domain.IntradaySnapshot, error)
}

type GetFundamentalsService interface {
	GetFundamentals(ctx context.Context, symbol string) (domain.Fundamentals, error)
}

type GetSymbolsService interface {
	GetSymbols(ctx context.Context, markets []string) ([]domain.Symbol, error)
}

type GetMarketsService interface {
	GetMarkets(ctx context.Context) ([]domain.Market, error)
}

type GetTimeframesService interface {
	GetTimeframes() []domain.TimeframeInfo
}
