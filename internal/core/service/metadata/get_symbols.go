package metadata

import (
	"context"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
)

type getSymbolsService struct {
	symbols out.SymbolRepository
}

func NewGetSymbolsService(symbols out.SymbolRepository) in.GetSymbolsService {
	return &getSymbolsService{symbols: symbols}
}

func (s *getSymbolsService) GetSymbols(ctx context.Context, markets []string) ([]domain.Symbol, error) {
	tracked, err := s.symbols.Tracked(ctx)
	if err != nil {
		return nil, err
	}
	if len(markets) == 0 {
		return tracked, nil
	}

	allowed := make(map[string]struct{}, len(markets))
	for _, m := range markets {
		allowed[m] = struct{}{}
	}
	filtered := make([]domain.Symbol, 0, len(tracked))
	for _, s := range tracked {
		if _, ok := allowed[s.Market]; ok {
			filtered = append(filtered, s)
		}
	}
	return filtered, nil
}
