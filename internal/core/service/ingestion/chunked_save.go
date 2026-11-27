package ingestion

import (
	"context"
	"errors"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

const (
	liveSaveChunkSize    = 5000
	minSaveChunkSize     = 500
	liveSaveChunkTimeout = 60 * time.Second
)

func (s *ingestCandlesService) currentChunkSize() int {
	if n := int(s.saveChunkSize.Load()); n > 0 {
		return n
	}
	return liveSaveChunkSize
}

func (s *ingestCandlesService) adjustChunkSize(grow bool) {
	for {
		raw := s.saveChunkSize.Load()
		cur := int(raw)
		if cur <= 0 {
			cur = liveSaveChunkSize
		}
		next := max(minSaveChunkSize, cur/2)
		if grow {
			next = min(liveSaveChunkSize, cur*2)
		}
		if next == cur || s.saveChunkSize.CompareAndSwap(raw, int64(next)) {
			return
		}
	}
}

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
	size := s.currentChunkSize()
	for i := 0; i < len(candles); {
		end := min(i+size, len(candles))
		if err := s.saveOneChunk(ctx, candles[i:end], timeout); err != nil {
			if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
				s.adjustChunkSize(false)
			}
			return candles[i:], err
		}
		i = end
	}
	if len(candles) >= size {
		s.adjustChunkSize(true)
	}
	return nil, nil
}
