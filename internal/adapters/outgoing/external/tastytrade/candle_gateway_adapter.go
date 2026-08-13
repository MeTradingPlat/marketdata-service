package tastytrade

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
)

var ProbeFromTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

var _ out.MarketDataGateway = (*Gateway)(nil)

type Gateway struct {
	oauth *OAuth
	pool  *CandlePool
}

func NewGateway(oauth *OAuth, pool *CandlePool) *Gateway {
	return &Gateway{oauth: oauth, pool: pool}
}

func (g *Gateway) GetCandles(ctx context.Context, symbol string, timeframe domain.Timeframe, from time.Time) ([]domain.Candle, error) {
	return g.pool.FetchHistory(ctx, symbol, timeframe, from)
}

// ProbeMaxDepth pide toda la profundidad real que TastyTrade entregue, sin
// el tope de 2000 velas que tenia el CandleCacheStore del servicio Java --
// confirmado que ese tope truncaba D1/M1 antes de la profundidad real.
func (g *Gateway) ProbeMaxDepth(ctx context.Context, symbol string, timeframe domain.Timeframe) ([]domain.Candle, error) {
	return g.pool.FetchHistory(ctx, symbol, timeframe, ProbeFromTime)
}

func (g *Gateway) SubscribeLiveCandles(ctx context.Context, symbol string, onCandle func(domain.Candle)) error {
	return g.pool.SubscribeLive(ctx, symbol, onCandle)
}
