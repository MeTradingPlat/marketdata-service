package metadata

import (
	"context"
	"strings"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
)

type getSymbolsService struct {
	symbols *SymbolsCache
}

func NewGetSymbolsService(symbols *SymbolsCache) in.GetSymbolsService {
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

	// El frontend manda los ids de /markets en minuscula (xnas, xnys...) y la
	// BD guarda los MICs en mayuscula (XNAS) -- normalizar ambos lados para
	// que el filtro case-insensitive matchee (confirmado en vivo el
	// 2026-08-19: el filtro por mercado devolvia vacio con el frontend).
	allowed := make(map[string]struct{}, len(markets))
	for _, m := range markets {
		allowed[strings.ToUpper(m)] = struct{}{}
	}
	filtered := make([]domain.Symbol, 0, len(tracked))
	for _, s := range tracked {
		if _, ok := allowed[strings.ToUpper(s.Market)]; ok {
			filtered = append(filtered, s)
		}
	}
	return filtered, nil
}
