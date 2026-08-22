package intraday

import (
	"context"
	"sync"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
)

const currentPricesWorkers = 20

// dbFallbackConcurrencyLimit acota, GLOBALMENTE (compartido entre TODAS las
// llamadas a GetCurrentPrices, sin importar cuantos escaneres o requests HTTP
// esten en vuelo a la vez), cuantas resolvePrice pueden tocar la BD al mismo
// tiempo. currentPricesWorkers ya acota el paralelismo DENTRO de un request,
// pero con varios escaneres pidiendo el universo completo a la vez eso no
// alcanza: 10 requests concurrentes x 20 workers cada uno pueden sumar 200
// intentos simultaneos contra un pool de 25 conexiones (confirmado en vivo:
// 5% de las llamadas a /marketdata/quotes/rest tardando ~15s por esa cola).
// Este semaforo es el limite real, independiente de cuantos escaneres existan
// o como cada cliente configure su propia concurrencia.
const dbFallbackConcurrencyLimit = 15

type getCurrentPricesService struct {
	repo    out.CandleRepository
	gateway out.MarketDataGateway
	tracker *SnapshotTracker
	dbSem   chan struct{}
}

func NewGetCurrentPricesService(repo out.CandleRepository, gateway out.MarketDataGateway, tracker *SnapshotTracker) in.GetCurrentPricesService {
	return &getCurrentPricesService{repo: repo, gateway: gateway, tracker: tracker, dbSem: make(chan struct{}, dbFallbackConcurrencyLimit)}
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

// resolvePrice prueba el tracker en memoria (LastClose) antes de tocar BD --
// mismo fallback que GetSnapshotsBatch, reusado aca: sin esto, un lote de
// quotes con muchos simbolos todavia no suscritos en vivo (justo tras un
// deploy/reconexion) pagaba una consulta por simbolo, el mismo problema ya
// resuelto para fundamentals/realtime (ver snapshot_tracker.go).
func (s *getCurrentPricesService) resolvePrice(ctx context.Context, symbol string) (float64, bool) {
	if current, ok := s.gateway.CurrentCandle(symbol); ok && current.Close != 0 {
		return current.Close, true
	}
	if price, _, ok := s.tracker.LastClose(symbol); ok {
		return price, true
	}

	select {
	case s.dbSem <- struct{}{}:
		defer func() { <-s.dbSem }()
	case <-ctx.Done():
		// El caller ya se rindio (timeout del lado de signal-processing) --
		// no vale la pena tomar un cupo del semaforo para un trabajo que
		// nadie va a leer.
		return 0, false
	}

	lastM1, err := s.repo.GetCandles(ctx, symbol, domain.M1, 1, nil)
	if err != nil || len(lastM1) == 0 {
		return 0, false
	}
	return lastM1[0].Close, true
}
