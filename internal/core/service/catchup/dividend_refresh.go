package catchup

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

// RefreshDividends trae dividend-amount/dividend-frequency para todo el
// universo rastreado desde /market-data/by-type (el gateway ya trocea y
// throttlea internamente) -- corre despues de D1/H1 en el barrido nocturno
// porque es puro REST/BD, no compite por conexiones DxLink como las fases
// de velas, no necesita su propia barrera estricta.
func RefreshDividends(ctx context.Context, gateway out.MarketDataGateway, repo out.FundamentalsRepository, tracked []domain.Symbol) {
	start := time.Now()
	symbols := make([]string, len(tracked))
	for i, s := range tracked {
		symbols[i] = s.Symbol
	}

	fundamentals, err := gateway.DividendInfo(ctx, symbols)
	if err != nil {
		log.Error().Err(err).Msg("dividend refresh failed")
		return
	}
	if err := repo.UpsertDividends(ctx, fundamentals); err != nil {
		log.Error().Err(err).Msg("upserting dividend refresh failed")
		return
	}

	log.Info().Int("symbols", len(fundamentals)).Dur("elapsed", time.Since(start)).Msg("dividend refresh finished")
}
