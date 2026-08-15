package metadata

import (
	"context"
	"strings"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
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
	symbols out.SymbolRepository
}

func NewGetMarketsService(symbols out.SymbolRepository) in.GetMarketsService {
	return &getMarketsService{symbols: symbols}
}

// GetMarkets solo devuelve mercados que de verdad tienen al menos un simbolo
// rastreado ahora mismo -- mismo "discovery" que hacia Java, en vez de una
// lista fija que podria anunciar un mercado sin nada real detras.
func (s *getMarketsService) GetMarkets(ctx context.Context) ([]domain.Market, error) {
	mics, err := s.symbols.Markets(ctx)
	if err != nil {
		return nil, err
	}
	markets := make([]domain.Market, 0, len(mics))
	for _, mic := range mics {
		name, ok := marketNames[mic]
		if !ok {
			name = mic
		}
		markets = append(markets, domain.Market{ID: strings.ToLower(mic), Nombre: name})
	}
	return markets, nil
}
