package ingestion

import (
	"context"
	"fmt"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/in"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

type ingestCandlesService struct {
	gateway out.MarketDataGateway
	repo    out.CandleRepository
}

func NewIngestCandlesService(gateway out.MarketDataGateway, repo out.CandleRepository) in.IngestCandlesService {
	return &ingestCandlesService{gateway: gateway, repo: repo}
}

func (s *ingestCandlesService) Backfill(ctx context.Context, symbol string, timeframe domain.Timeframe) error {
	candles, err := s.gateway.ProbeMaxDepth(ctx, symbol, timeframe)
	if err != nil {
		return fmt.Errorf("probing max depth for %s %s: %w", symbol, timeframe, err)
	}
	if len(candles) == 0 {
		return nil
	}
	if err := s.repo.Save(ctx, candles); err != nil {
		return fmt.Errorf("saving backfilled candles for %s %s: %w", symbol, timeframe, err)
	}
	return nil
}

// SubscribeLiveCandles solo invoca el callback con velas ya cerradas -- ver
// MarketDataGateway, el merge de ticks parciales es responsabilidad del
// adaptador, no de este use case.
func (s *ingestCandlesService) StreamLive(ctx context.Context, symbol string) error {
	return s.gateway.SubscribeLiveCandles(ctx, symbol, func(c domain.Candle) {
		if err := s.repo.Save(ctx, []domain.Candle{c}); err != nil {
			log.Error().Err(err).Str("symbol", symbol).Msg("failed to save live candle")
		}
	})
}
