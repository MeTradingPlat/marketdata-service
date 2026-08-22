package fundamentals

import (
	"context"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
)

type getFundamentalsRealtimeService struct {
	fundamentals *FundamentalsCache
	symbols      out.SymbolRepository
	intraday     in.GetIntradaySnapshotService
}

func NewGetFundamentalsRealtimeService(fundamentals *FundamentalsCache, symbols out.SymbolRepository, intraday in.GetIntradaySnapshotService) in.GetFundamentalsRealtimeService {
	return &getFundamentalsRealtimeService{fundamentals: fundamentals, symbols: symbols, intraday: intraday}
}

// GetFundamentalsRealtime junta tres fuentes por lote, cada una en un
// numero fijo de queries (ver GetSnapshotsBatch para por que
// IntradaySnapshot dejo de pedirse simbolo por simbolo). Un simbolo sin
// fundamentals Y sin symbol tracked no aparece en el resultado -- no hay
// nada real que devolverle a signal-processing-service para el.
func (s *getFundamentalsRealtimeService) GetFundamentalsRealtime(ctx context.Context, symbols []string) map[string]dto.FundamentalRealtime {
	fundamentalsBySymbol := s.fundamentals.GetBatch(symbols)
	equitiesBySymbol, err := s.symbols.GetBatch(ctx, symbols)
	if err != nil {
		equitiesBySymbol = map[string]domain.Symbol{}
	}

	// knownPrevCloses tambien cubre "se intento y no hay dato" (PrevClose
	// nil pero PrevCloseUpdatedAt no-nil, ver el comentario del campo en
	// domain.Fundamentals) con 0 -- sin esto, esos simbolos volvian a pagar
	// la busqueda de 10 dias en CADA request pese a que la ventana de
	// mantenimiento ya determino que no tienen M1.
	knownPrevCloses := make(map[string]float64, len(fundamentalsBySymbol))
	for symbol, f := range fundamentalsBySymbol {
		if f.PrevClose != nil {
			knownPrevCloses[symbol] = *f.PrevClose
		} else if f.PrevCloseUpdatedAt != nil {
			knownPrevCloses[symbol] = 0
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
