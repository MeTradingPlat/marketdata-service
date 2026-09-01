package tastytrade

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
)

var ProbeFromTime = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

var _ out.MarketDataGateway = (*Gateway)(nil)
var _ out.OpenInterestGateway = (*Gateway)(nil)

type Gateway struct {
	oauth *OAuth
	pool  *CandlePool

	openInterestCache openInterestCache
}

func NewGateway(oauth *OAuth, pool *CandlePool) *Gateway {
	return &Gateway{oauth: oauth, pool: pool, openInterestCache: openInterestCache{entries: make(map[string]openInterestEntry)}}
}

func (g *Gateway) GetCandles(ctx context.Context, symbol string, timeframe domain.Timeframe, from time.Time) ([]domain.Candle, error) {
	return g.pool.FetchHistory(ctx, symbol, timeframe, from)
}

func (g *Gateway) GetCandlesBatch(ctx context.Context, timeframe domain.Timeframe, froms map[string]time.Time) (map[string][]domain.Candle, error) {
	return g.pool.FetchHistoryBatch(ctx, timeframe, froms)
}

// ProbeMaxDepth pide toda la profundidad real que TastyTrade entregue, sin
// el tope de 2000 velas que tenia el CandleCacheStore del servicio Java --
// confirmado que ese tope truncaba D1/M1 antes de la profundidad real.
// Usa FetchHistoryDeep: sin watermark que acote, la rafaga historica de un
// simbolo profundo puede superar la espera del fetch incremental (ver
// historyDeepWait en candle_pool.go) y quedar truncada para siempre.
func (g *Gateway) ProbeMaxDepth(ctx context.Context, symbol string, timeframe domain.Timeframe) ([]domain.Candle, error) {
	return g.pool.FetchHistoryDeep(ctx, symbol, timeframe, ProbeFromTime)
}

// ProbeMaxDepthWithWait es ProbeMaxDepth con un wait a eleccion -- solo para
// el diagnostico de profundidad (ver debug_probe_handler.go). NO forma
// parte de out.MarketDataGateway a proposito: agregarla ahi obligaria a
// implementarla en cada fake de test de ese puerto para una funcion que
// ningun flujo de produccion real necesita, solo este diagnostico puntual.
func (g *Gateway) ProbeMaxDepthWithWait(ctx context.Context, symbol string, timeframe domain.Timeframe, wait time.Duration) ([]domain.Candle, error) {
	return g.pool.FetchHistoryWithWait(ctx, symbol, timeframe, ProbeFromTime, wait)
}

// SubscribeLiveCandles trata "from" en cero (sin watermark todavia, simbolo
// nuevo) igual que ProbeMaxDepth: pide toda la profundidad M1 real que
// TastyTrade entregue en vez de arrancar la suscripcion en vivo sin nada de
// historial detras. onClosed recibe cada vela M1 al cerrar; onTick recibe la
// vela en formacion despues de CADA tick (la que dibuja el grafico en vivo).
func (g *Gateway) SubscribeLiveCandles(ctx context.Context, symbol string, from time.Time, onClosed func(domain.Candle), onTick func(domain.Candle)) error {
	if from.IsZero() {
		from = ProbeFromTime
	}
	return g.pool.SubscribeLive(ctx, symbol, from, onClosed, onTick)
}

func (g *Gateway) CurrentCandle(symbol string) (domain.Candle, bool) {
	return g.pool.CurrentCandle(symbol)
}

func (g *Gateway) LiveSubscribed(symbol string) bool {
	return g.pool.LiveSubscribed(symbol)
}

func (g *Gateway) ResetLiveConnections() {
	g.pool.CloseAllConnections()
}
