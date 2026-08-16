package metadata

import (
	"context"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
)

type getSymbolDetailsService struct {
	symbols      out.SymbolRepository
	fundamentals out.FundamentalsRepository
}

func NewGetSymbolDetailsService(symbols out.SymbolRepository, fundamentals out.FundamentalsRepository) in.GetSymbolDetailsService {
	return &getSymbolDetailsService{symbols: symbols, fundamentals: fundamentals}
}

func (s *getSymbolDetailsService) GetSymbolDetails(ctx context.Context, symbol string) (dto.SymbolDetails, error) {
	equity, err := s.symbols.GetBySymbol(ctx, symbol)
	if err != nil {
		return dto.SymbolDetails{}, err
	}

	fundamentals, err := s.fundamentals.Get(ctx, symbol)
	if err != nil {
		return dto.SymbolDetails{}, err
	}

	return dto.SymbolDetails{ActiveEquity: equity, FundamentalData: fundamentals}, nil
}
