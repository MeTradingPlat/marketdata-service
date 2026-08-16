package fundamentals

import (
	"context"
	"sync"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
)

const realtimeSnapshotWorkers = 20

type getFundamentalsRealtimeService struct {
	fundamentals out.FundamentalsRepository
	symbols      out.SymbolRepository
	intraday     in.GetIntradaySnapshotService
}

func NewGetFundamentalsRealtimeService(fundamentals out.FundamentalsRepository, symbols out.SymbolRepository, intraday in.GetIntradaySnapshotService) in.GetFundamentalsRealtimeService {
	return &getFundamentalsRealtimeService{fundamentals: fundamentals, symbols: symbols, intraday: intraday}
}

// GetFundamentalsRealtime junta tres fuentes por lote: Fundamentals y
// Symbol salen de una sola consulta cada una (symbol = ANY(...)), pero
// IntradaySnapshot no tiene version en lote todavia (agrega M1 de hoy por
// simbolo) asi que se pide con concurrencia acotada, mismo criterio que
// GetCandlesBatch/GetCurrentPrices. Un simbolo sin fundamentals Y sin
// symbol tracked no aparece en el resultado -- no hay nada real que
// devolverle a signal-processing-service para el.
func (s *getFundamentalsRealtimeService) GetFundamentalsRealtime(ctx context.Context, symbols []string) map[string]dto.FundamentalRealtime {
	fundamentalsBySymbol, err := s.fundamentals.GetBatch(ctx, symbols)
	if err != nil {
		fundamentalsBySymbol = map[string]domain.Fundamentals{}
	}
	equitiesBySymbol, err := s.symbols.GetBatch(ctx, symbols)
	if err != nil {
		equitiesBySymbol = map[string]domain.Symbol{}
	}
	snapshotsBySymbol := s.fetchSnapshots(ctx, symbols)

	result := make(map[string]dto.FundamentalRealtime, len(symbols))
	for _, symbol := range symbols {
		equity, hasEquity := equitiesBySymbol[symbol]
		fund, hasFund := fundamentalsBySymbol[symbol]
		if !hasEquity && !hasFund {
			continue
		}
		result[symbol] = toRealtime(symbol, equity, fund, snapshotsBySymbol[symbol])
	}
	return result
}

func (s *getFundamentalsRealtimeService) fetchSnapshots(ctx context.Context, symbols []string) map[string]domain.IntradaySnapshot {
	result := make(map[string]domain.IntradaySnapshot, len(symbols))
	var mu sync.Mutex

	jobs := make(chan string, len(symbols))
	for _, sym := range symbols {
		jobs <- sym
	}
	close(jobs)

	var wg sync.WaitGroup
	for i := 0; i < realtimeSnapshotWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for symbol := range jobs {
				snap, err := s.intraday.GetSnapshot(ctx, symbol)
				if err != nil {
					continue
				}
				mu.Lock()
				result[symbol] = snap
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return result
}
