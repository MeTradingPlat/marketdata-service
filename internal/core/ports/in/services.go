package in

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
)

type IngestCandlesService interface {
	Backfill(ctx context.Context, symbol string, timeframe domain.Timeframe) error
	StreamLive(ctx context.Context, symbol string) error
	RetryPendingSaves(ctx context.Context)
}

type GetCandlesService interface {
	// before es nil para las barras mas recientes, o un limite exclusivo
	// para paginar hacia atras (scroll a la izquierda del chart).
	GetCandles(ctx context.Context, symbol string, timeframe domain.Timeframe, bars int, before *time.Time) ([]domain.Candle, error)
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

type SearchSymbolsService interface {
	Search(ctx context.Context, query string, markets []string, page, size int) (dto.PaginatedResponse[domain.Symbol], error)
}

type GetSymbolDetailsService interface {
	GetSymbolDetails(ctx context.Context, symbol string) (dto.SymbolDetails, error)
}

type GetMarketsService interface {
	GetMarkets(ctx context.Context) ([]dto.Market, error)
}

type GetTimeframesService interface {
	GetTimeframes() []dto.TimeframeInfo
}
