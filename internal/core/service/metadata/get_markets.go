package metadata

import (
	"context"
	"strings"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
)

// marketNames son los mismos nombres amigables que ya usaba MetadataController
// en Java (TastyTrade no los expone en ningun listado propio).
var marketNames = map[string]string{
	"XNAS": "NASDAQ",
	"XNYS": "NYSE",
	"XASE": "AMEX",
	"ARCX": "NYSE ARCA (ETFs)",
	"BATS": "CBOE BZX (ETFs)",
	"OTC":  "OTC Markets",
}

type getMarketsService struct {
	symbols *SymbolsCache
}

func NewGetMarketsService(symbols *SymbolsCache) in.GetMarketsService {
	return &getMarketsService{symbols: symbols}
}

// GetMarkets solo devuelve mercados que de verdad tienen al menos un simbolo
// rastreado ahora mismo -- mismo "discovery" que hacia Java, en vez de una
// lista fija que podria anunciar un mercado sin nada real detras.
func (s *getMarketsService) GetMarkets(ctx context.Context) ([]dto.Market, error) {
	mics, err := s.symbols.Markets(ctx)
	if err != nil {
		return nil, err
	}
	markets := make([]dto.Market, 0, len(mics))
	for _, mic := range mics {
		name, ok := marketNames[mic]
		if !ok {
			name = mic
		}
		markets = append(markets, dto.Market{ID: strings.ToLower(mic), Nombre: name})
	}
	return markets, nil
}
