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
	IsLive(symbol string) bool
	IsAttempted(symbol string) bool
}

type GetCandlesService interface {
	// before es nil para las barras mas recientes, o un limite exclusivo
	// para paginar hacia atras (scroll a la izquierda del chart).
	GetCandles(ctx context.Context, symbol string, timeframe domain.Timeframe, bars int, before *time.Time) ([]domain.Candle, error)
}

// GetCurrentCandleService devuelve la vela EN FORMACION del simbolo+timeframe
// (plana al ultimo cierre si el periodo no tiene datos). Exclusiva del
// grafico en vivo -- signal-processing consume velas cerradas y no pasa
// por aca; la vela en formacion nunca se guarda.
type GetCurrentCandleService interface {
	GetCurrentCandle(ctx context.Context, symbol string, timeframe domain.Timeframe) (*dto.CandleBar, error)
}

type GetIntradaySnapshotService interface {
	GetSnapshot(ctx context.Context, symbol string) (domain.IntradaySnapshot, error)
	// GetSnapshotsBatch es GetSnapshot para un lote -- ver
	// out.CandleRepository.GetIntradaySessionsBatch sobre por que
	// fundamentals/realtime necesita esto en vez de N llamadas a GetSnapshot.
	GetSnapshotsBatch(ctx context.Context, symbols []string) map[string]domain.IntradaySnapshot
}

// GetCurrentPricesService es la version liviana de GetIntradaySnapshotService
// para lotes -- solo precio, sin sesiones del dia. Un simbolo sin precio
// disponible queda afuera del mapa devuelto, nunca en 0.
type GetCurrentPricesService interface {
	GetCurrentPrices(ctx context.Context, symbols []string) map[string]float64
}

type GetFundamentalsService interface {
	GetFundamentals(ctx context.Context, symbol string) (domain.Fundamentals, error)
}

// GetFundamentalsRealtimeService es el contrato en lote que consume
// signal-processing-service (POST /marketdata/fundamentals/realtime) --
// un simbolo sin cobertura real simplemente no aparece en el mapa.
type GetFundamentalsRealtimeService interface {
	GetFundamentalsRealtime(ctx context.Context, symbols []string) map[string]dto.FundamentalRealtime
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
