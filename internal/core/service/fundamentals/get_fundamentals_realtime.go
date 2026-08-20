package fundamentals

import (
	"context"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
)

type getFundamentalsRealtimeService struct {
	fundamentals out.FundamentalsRepository
	symbols      out.SymbolRepository
	intraday     in.GetIntradaySnapshotService
}

func NewGetFundamentalsRealtimeService(fundamentals out.FundamentalsRepository, symbols out.SymbolRepository, intraday in.GetIntradaySnapshotService) in.GetFundamentalsRealtimeService {
	return &getFundamentalsRealtimeService{fundamentals: fundamentals, symbols: symbols, intraday: intraday}
}

// GetFundamentalsRealtime junta tres fuentes por lote, cada una en un
// numero fijo de queries (ver GetSnapshotsBatch para por que
// IntradaySnapshot dejo de pedirse simbolo por simbolo). Un simbolo sin
// fundamentals Y sin symbol tracked no aparece en el resultado -- no hay
// nada real que devolverle a signal-processing-service para el.
func (s *getFundamentalsRealtimeService) GetFundamentalsRealtime(ctx context.Context, symbols []string) map[string]dto.FundamentalRealtime {
	fundamentalsBySymbol, err := s.fundamentals.GetBatch(ctx, symbols)
	if err != nil {
		fundamentalsBySymbol = map[string]domain.Fundamentals{}
	}
	equitiesBySymbol, err := s.symbols.GetBatch(ctx, symbols)
	if err != nil {
		equitiesBySymbol = map[string]domain.Symbol{}
	}

	knownPrevCloses := make(map[string]float64, len(fundamentalsBySymbol))
	for symbol, f := range fundamentalsBySymbol {
		if f.PrevClose != nil {
			knownPrevCloses[symbol] = *f.PrevClose
		}
	}
	snapshotsBySymbol := s.intraday.GetSnapshotsBatch(ctx, symbols, knownPrevCloses)

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
