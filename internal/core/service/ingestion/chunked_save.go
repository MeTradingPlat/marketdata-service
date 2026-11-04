package ingestion

import (
	"context"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

const liveSaveChunkSize = 5000

func (s *ingestCandlesService) saveInChunks(ctx context.Context, candles []domain.Candle) ([]domain.Candle, error) {
	for i := 0; i < len(candles); i += liveSaveChunkSize {
		end := i + liveSaveChunkSize
		if end > len(candles) {
			end = len(candles)
		}
		chunk := candles[i:end]
		if err := s.repo.Save(ctx, chunk, false); err != nil {
			return candles[i:], err
		}
	}
	return nil, nil
}
