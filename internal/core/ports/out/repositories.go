package out

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

type CandleRepository interface {
	Save(ctx context.Context, candles []domain.Candle) error
	// before es nil para las barras mas recientes, o un limite exclusivo
	// para paginar hacia atras (ver CandleRepository.GetCandles).
	GetCandles(ctx context.Context, symbol string, timeframe domain.Timeframe, bars int, before *time.Time) ([]domain.Candle, error)
	GetWatermark(ctx context.Context, symbol string, timeframe domain.Timeframe) (newest *time.Time, err error)
	SymbolsWithData(ctx context.Context, timeframe domain.Timeframe) (map[string]struct{}, error)
	// GetIntradaySessions agrega las velas M1 de hoy por sesion (pre-market,
	// regular, post-market) -- devuelve solo esos campos de
	// IntradaySnapshot, el resto (precio/volumen actual, prevClose) los
	// completa el caso de uso desde otras fuentes.
	GetIntradaySessions(ctx context.Context, symbol string) (domain.IntradaySnapshot, error)
}

type SymbolRepository interface {
	Upsert(ctx context.Context, symbols []domain.Symbol) error
	Tracked(ctx context.Context) ([]domain.Symbol, error)
	GetBySymbol(ctx context.Context, symbol string) (domain.Symbol, error)
	// Search ordena por last_volume descendente (ver Save() en el
	// repositorio) en vez de alfabetico -- mas relevante para un screener.
	Search(ctx context.Context, query string, markets []string, page, size int) (symbols []domain.Symbol, totalElements int64, err error)
	Deactivate(ctx context.Context, symbols []string) error
	Markets(ctx context.Context) ([]string, error)
	// TopSymbolsPerMarket devuelve hasta n simbolos por mercado, ordenados
	// por last_volume descendente -- universo acotado para el refresco de
	// fundamentales via REST (no todo el universo rastreado de una).
	TopSymbolsPerMarket(ctx context.Context, n int) ([]domain.Symbol, error)
}

type FundamentalsRepository interface {
	Get(ctx context.Context, symbol string) (domain.Fundamentals, error)
	UpsertDividends(ctx context.Context, fundamentals []domain.Fundamentals) error
	UpsertMarketMetrics(ctx context.Context, fundamentals []domain.Fundamentals) error
}
