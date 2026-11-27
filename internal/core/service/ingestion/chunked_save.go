package ingestion

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

const (
	liveSaveChunkSize    = 5000
	liveSaveChunkTimeout = 60 * time.Second
)

func (s *ingestCandlesService) chunkTimeout() time.Duration {
	if s.saveChunkTimeout > 0 {
		return s.saveChunkTimeout
	}
	return liveSaveChunkTimeout
}

func (s *ingestCandlesService) saveOneChunk(ctx context.Context, chunk []domain.Candle, timeout time.Duration) error {
	chunkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return s.repo.Save(chunkCtx, chunk, false)
}

func (s *ingestCandlesService) saveInChunks(ctx context.Context, candles []domain.Candle) ([]domain.Candle, error) {
	timeout := s.chunkTimeout()
	for i := 0; i < len(candles); i += liveSaveChunkSize {
		end := min(i+liveSaveChunkSize, len(candles))
		if err := s.saveOneChunk(ctx, candles[i:end], timeout); err != nil {
			return candles[i:], err
		}
	}
	return nil, nil
}
