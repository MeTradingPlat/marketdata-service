package intraday

import (
	"context"
	"sync"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
)

const currentPricesWorkers = 20

type getCurrentPricesService struct {
	repo    out.CandleRepository
	gateway out.MarketDataGateway
}

func NewGetCurrentPricesService(repo out.CandleRepository, gateway out.MarketDataGateway) in.GetCurrentPricesService {
	return &getCurrentPricesService{repo: repo, gateway: gateway}
}

// GetCurrentPrices es la version liviana de GetSnapshot -- solo el precio,
// para /marketdata/quotes/rest (signal-processing-service pide esto para un
// lote ya filtrado por fundamentales, necesita el precio mas fresco posible
// sin pagar de nuevo el costo de las sesiones del dia). Mismo criterio de
// "vela en formacion primero, ultima M1 cerrada si no hay" que GetSnapshot.
// Un simbolo sin ningun precio disponible queda afuera del mapa, no en 0.
func (s *getCurrentPricesService) GetCurrentPrices(ctx context.Context, symbols []string) map[string]float64 {
	result := make(map[string]float64, len(symbols))
	var mu sync.Mutex

	jobs := make(chan string, len(symbols))
	for _, sym := range symbols {
		jobs <- sym
	}
	close(jobs)

	var wg sync.WaitGroup
	for i := 0; i < currentPricesWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for symbol := range jobs {
				price, ok := s.resolvePrice(ctx, symbol)
				if !ok {
					continue
				}
				mu.Lock()
				result[symbol] = price
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return result
}

func (s *getCurrentPricesService) resolvePrice(ctx context.Context, symbol string) (float64, bool) {
	if current, ok := s.gateway.CurrentCandle(symbol); ok && current.Close != 0 {
		return current.Close, true
	}
	lastM1, err := s.repo.GetCandles(ctx, symbol, domain.M1, 1, nil)
	if err != nil || len(lastM1) == 0 {
		return 0, false
	}
	return lastM1[0].Close, true
}
