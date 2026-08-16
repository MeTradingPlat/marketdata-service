package metadata

import (
	"context"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain/dto"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
)

type searchSymbolsService struct {
	symbols out.SymbolRepository
}

func NewSearchSymbolsService(symbols out.SymbolRepository) in.SearchSymbolsService {
	return &searchSymbolsService{symbols: symbols}
}

func (s *searchSymbolsService) Search(ctx context.Context, query string, markets []string, page, size int) (dto.PaginatedResponse[domain.Symbol], error) {
	results, total, err := s.symbols.Search(ctx, query, markets, page, size)
	if err != nil {
		return dto.PaginatedResponse[domain.Symbol]{}, err
	}

	totalPages := 0
	if size > 0 {
		totalPages = int((total + int64(size) - 1) / int64(size))
	}

	return dto.PaginatedResponse[domain.Symbol]{
		Data:          results,
		Page:          page,
		PageSize:      size,
		TotalPages:    totalPages,
		TotalElements: total,
	}, nil
}
