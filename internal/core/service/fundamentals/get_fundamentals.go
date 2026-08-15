package fundamentals

import (
	"context"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
)

type getFundamentalsService struct {
	repo out.FundamentalsRepository
}

func NewGetFundamentalsService(repo out.FundamentalsRepository) in.GetFundamentalsService {
	return &getFundamentalsService{repo: repo}
}

func (s *getFundamentalsService) GetFundamentals(ctx context.Context, symbol string) (domain.Fundamentals, error) {
	return s.repo.Get(ctx, symbol)
}
