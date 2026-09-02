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
	// RetryPendingSaves intenta guardar lo que quedo bufferizado -- devuelve
	// false solo si habia algo pendiente y el intento fallo (para que el
	// caller sepa cuando hacer backoff), true si no habia nada pendiente o
	// si se guardo todo.
	RetryPendingSaves(ctx context.Context) bool
	IsLive(symbol string) bool
	IsAttempted(symbol string) bool
}

type GetCandlesService interface {
	// before es nil para las barras mas recientes, o un limite exclusivo
	// para paginar hacia atras (scroll a la izquierda del chart).
	GetCandles(ctx context.Context, symbol string, timeframe domain.Timeframe, bars int, before *time.Time) ([]domain.Candle, error)
	// GetCandlesBatch es GetCandles para un lote -- para timeframes con
	// continuous aggregate (M5/M15) usa una sola consulta en vez de una por
	// simbolo (ver out.CandleRepository.GetSeriesAggregatedBatch); el resto
	// de timeframes derivados sigue con concurrencia acotada por simbolo.
	// Un simbolo sin ninguna vela disponible no aparece en el mapa.
	GetCandlesBatch(ctx context.Context, symbols []string, timeframe domain.Timeframe, bars int) map[string][]domain.Candle
}

// GetCurrentCandleService devuelve la vela EN FORMACION del simbolo+timeframe,
// o nil si el periodo todavia no tiene ningun tick real. Exclusiva del
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
	// knownPrevCloses es el prevClose YA calculado por RefreshPrevClose (ver
	// domain.Fundamentals.PrevClose) para los simbolos que el caller ya
	// tiene a mano -- evita recalcular la misma consulta de subasta/D1 para
	// un simbolo cuyo prevClose de hoy ya se sabe.
	GetSnapshotsBatch(ctx context.Context, symbols []string, knownPrevCloses map[string]float64) map[string]domain.IntradaySnapshot
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
