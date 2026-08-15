package out

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// MarketDataGateway es el unico puerto que conoce a un proveedor de datos
// externo. Cambiar de TastyTrade a otro proveedor = un adaptador nuevo que
// implemente esta interfaz, sin tocar core/.
type MarketDataGateway interface {
	GetCandles(ctx context.Context, symbol string, timeframe domain.Timeframe, from time.Time) ([]domain.Candle, error)
	ProbeMaxDepth(ctx context.Context, symbol string, timeframe domain.Timeframe) ([]domain.Candle, error)
	SubscribeLiveCandles(ctx context.Context, symbol string, from time.Time, onCandle func(domain.Candle)) error
	ActiveSymbols(ctx context.Context) ([]domain.Symbol, error)
	CurrentCandle(symbol string) (domain.Candle, bool)
	DividendInfo(ctx context.Context, symbols []string) ([]domain.Fundamentals, error)
}
