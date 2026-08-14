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

	// AggregateH1/AggregateD1 derivan hacia adelante desde M1 ya guardado
	// -- reemplazan a los continuous aggregates nativos de TimescaleDB para
	// no tener que dividir la lectura entre varias tablas/vistas segun la
	// temporalidad; escriben en la misma tabla candles via UPSERT.
	AggregateH1(ctx context.Context) error
	AggregateD1(ctx context.Context) error
}

type SymbolRepository interface {
	Upsert(ctx context.Context, symbols []domain.Symbol) error
	Tracked(ctx context.Context) ([]domain.Symbol, error)
}
