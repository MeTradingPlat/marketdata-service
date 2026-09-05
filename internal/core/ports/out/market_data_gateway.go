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
	// GetCandlesBatch pide historial para muchos simbolos en UNA sola
	// suscripcion DxLink (con su propio fromTime cada uno) -- es el
	// agrupamiento original del pool de Java (100 simbolos por canal) que
	// el barrido usa para no pagar 13k round-trips de add/remove por
	// timeframe (confirmado en vivo: una suscripcion por simbolo hacia el
	// barrido 26k mensajes y 17 minutos -- en lotes son ~130 mensajes y
	// segundos).
	GetCandlesBatch(ctx context.Context, timeframe domain.Timeframe, froms map[string]time.Time) (map[string][]domain.Candle, error)
	ProbeMaxDepth(ctx context.Context, symbol string, timeframe domain.Timeframe) ([]domain.Candle, error)
	// SubscribeLiveCandles abre la suscripcion M1 en vivo del simbolo:
	// onClosed se invoca con cada vela al cerrar, onTick con la vela en
	// formacion despues de cada tick (siempre que tenga OHLC completo).
	SubscribeLiveCandles(ctx context.Context, symbol string, from time.Time, onClosed func(domain.Candle), onTick func(domain.Candle)) error
	ActiveSymbols(ctx context.Context) ([]domain.Symbol, error)
	CurrentCandle(symbol string) (domain.Candle, bool)
	// LiveSubscribed dice si el stream M1 en vivo del simbolo esta
	// ACTUALMENTE registrado en el pool -- el reconciliador lo usa para
	// detectar muertes silenciosas (un resubscribe fallido tras una
	// reconexion deja el stream mudo sin error visible, confirmado en vivo
	// el 2026-08-18 con OSRH).
	LiveSubscribed(symbol string) bool
	DividendInfo(ctx context.Context, symbols []string) ([]domain.Fundamentals, error)
	// MarketMetrics trae market-cap/beta/liquidez/IV/proximo earnings --
	// fuente separada de DividendInfo (otro endpoint REST), se llama aparte.
	MarketMetrics(ctx context.Context, symbols []string) ([]domain.Fundamentals, error)
	// EarningsReports trae el historial de reportes de earnings REALES (con
	// EPS ya publicado) de un simbolo -- endpoint separado, por-simbolo, sin
	// batch. Usado para saber la fecha del ultimo reporte y predecir el
	// proximo cuando MarketMetrics no trae uno vigente.
	EarningsReports(ctx context.Context, symbol string) ([]domain.EarningsReportItem, error)
	// ResetLiveConnections cierra todas las conexiones DxLink pooled -- se
	// llama entre fases del barrido (D1->H1->M1) para que ninguna arrastre
	// sesiones de la fase anterior justo cuando la siguiente abre varias
	// de golpe (ver CandlePool.CloseAllConnections).
	ResetLiveConnections()
	// RefreshLiveSubscriptions reenvia el Add M1 de cada simbolo ya
	// suscrito por su MISMO canal -- nunca abre canal o conexion nueva (ver
	// CandlePool.RefreshLiveSubscriptions). Se llama cada minuto, solo
	// fuera del fill/refill, como red de seguridad contra una suscripcion
	// que dejo de empujar datos en silencio sin que el canal ni la conexion
	// se enteraran.
	RefreshLiveSubscriptions(ctx context.Context)
}
