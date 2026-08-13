package ingestion

import (
	"context"
	"fmt"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
)

type getCandlesService struct {
	repo out.CandleRepository
}

func NewGetCandlesService(repo out.CandleRepository) in.GetCandlesService {
	return &getCandlesService{repo: repo}
}

func (s *getCandlesService) GetCandles(ctx context.Context, symbol string, timeframe domain.Timeframe, bars int) ([]domain.Candle, error) {
	candles, err := s.repo.GetCandles(ctx, symbol, timeframe, bars)
	if err != nil {
		return nil, fmt.Errorf("getting candles for %s %s: %w", symbol, timeframe, err)
	}
	return candles, nil
}
