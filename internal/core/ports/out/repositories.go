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
	// GetIntradaySessions agrega las velas M1 de hoy por sesion (pre-market,
	// regular, post-market) -- devuelve solo esos campos de
	// IntradaySnapshot, el resto (precio/volumen actual, prevClose) los
	// completa el caso de uso desde otras fuentes.
	GetIntradaySessions(ctx context.Context, symbol string) (domain.IntradaySnapshot, error)
}

type SymbolRepository interface {
	Upsert(ctx context.Context, symbols []domain.Symbol) error
	Tracked(ctx context.Context) ([]domain.Symbol, error)
	Deactivate(ctx context.Context, symbols []string) error
}
