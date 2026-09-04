package intraday

import (
	"context"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
)

type getCurrentPricesService struct {
	repo    out.CandleRepository
	gateway out.MarketDataGateway
	tracker *SnapshotTracker
}

func NewGetCurrentPricesService(repo out.CandleRepository, gateway out.MarketDataGateway, tracker *SnapshotTracker) in.GetCurrentPricesService {
	return &getCurrentPricesService{repo: repo, gateway: gateway, tracker: tracker}
}

// GetCurrentPrices es la version liviana de GetSnapshot -- solo el precio,
// para /marketdata/quotes/rest (signal-processing-service pide esto para un
// lote ya filtrado por fundamentales, necesita el precio mas fresco posible
// sin pagar de nuevo el costo de las sesiones del dia). Mismo criterio de
// "vela en formacion primero, ultima M1 cerrada si no hay" que GetSnapshot.
// Un simbolo sin ningun precio disponible queda afuera del mapa, no en 0.
//
// El fallback a BD es UN SOLO GetSeriesPriority para todo el lote que no
// resolvio en memoria -- antes era un GetCandles por simbolo (hasta 15 en
// paralelo via semaforo), y con varios escaneres pidiendo el universo
// completo a la vez eso sumaba decenas de queries individuales contra
// Postgres en la apertura del mercado (confirmado en vivo el 2026-09-04:
// contribuyo a un load average de 160+ y checkpoints de 270s+ escribiendo
// casi nada, mismo patron que GetSnapshotsBatch ya resolvio para su propio
// fallback M1, ver needM1Fallback en get_intraday_snapshot.go).
func (s *getCurrentPricesService) GetCurrentPrices(ctx context.Context, symbols []string) map[string]float64 {
	result := make(map[string]float64, len(symbols))
	needDB := make([]string, 0, len(symbols))

	for _, symbol := range symbols {
		if current, ok := s.gateway.CurrentCandle(symbol); ok && current.Close != 0 {
			result[symbol] = current.Close
			continue
		}
		if price, _, ok := s.tracker.LastClose(symbol); ok {
			result[symbol] = price
			continue
		}
		needDB = append(needDB, symbol)
	}

	if len(needDB) == 0 {
		return result
	}
	series, err := s.repo.GetSeriesPriority(ctx, needDB, domain.M1, 1)
	if err != nil {
		return result
	}
	for symbol, candles := range series {
		if len(candles) > 0 {
			result[symbol] = candles[0].Close
		}
	}
	return result
}
