package catchup

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

// RefreshTradingStatus trae dividendos + halt/trading-status para todo el
// universo rastreado desde /market-data/by-type -- lo unico de todo lo que
// trae este paquete que de verdad necesita estar al dia en minutos, no de
// una noche para otra (un halt es un evento acotado en el tiempo, si se
// entera reciendo la manana siguiente ya no sirve de nada). Se llama tanto
// desde el barrido nocturno como desde un loop propio mas frecuente (ver
// cmd/api/trading_status_loop.go).
func RefreshTradingStatus(ctx context.Context, gateway out.MarketDataGateway, symbolsRepo out.SymbolRepository, fundamentalsRepo out.FundamentalsRepository) {
	tracked, err := symbolsRepo.Tracked(ctx)
	if err != nil {
		log.Error().Err(err).Msg("trading status refresh: failed to list tracked symbols")
		return
	}
	symbols := toSymbolStrings(tracked)

	start := time.Now()
	dividends, err := gateway.DividendInfo(ctx, symbols)
	if err != nil {
		log.Error().Err(err).Msg("dividend/halt refresh failed")
		return
	}
	if err := fundamentalsRepo.UpsertDividends(ctx, dividends); err != nil {
		log.Error().Err(err).Msg("upserting dividend/halt refresh failed")
		return
	}
	log.Info().Int("symbols", len(symbols)).Dur("elapsed", time.Since(start)).Msg("trading status refresh finished")
}

// RefreshMarketMetrics trae market-cap/beta/liquidez/IV/proximo earnings
// para todo el universo rastreado desde /market-metrics -- datos de fondo
// que TastyTrade mismo solo actualiza de su lado unas pocas veces al dia
// (ver updated-at en las respuestas reales), un refresco nocturno ya
// alcanza y sobra. Corre despues de D1/H1 en el barrido nocturno porque es
// puro REST/BD, no compite por conexiones DxLink con las fases de velas.
func RefreshMarketMetrics(ctx context.Context, gateway out.MarketDataGateway, symbolsRepo out.SymbolRepository, fundamentalsRepo out.FundamentalsRepository) {
	tracked, err := symbolsRepo.Tracked(ctx)
	if err != nil {
		log.Error().Err(err).Msg("market metrics refresh: failed to list tracked symbols")
		return
	}
	symbols := toSymbolStrings(tracked)

	start := time.Now()
	metrics, err := gateway.MarketMetrics(ctx, symbols)
	if err != nil {
		log.Error().Err(err).Msg("market metrics refresh failed")
		return
	}
	if err := fundamentalsRepo.UpsertMarketMetrics(ctx, metrics); err != nil {
		log.Error().Err(err).Msg("upserting market metrics refresh failed")
		return
	}
	log.Info().Int("symbols", len(symbols)).Dur("elapsed", time.Since(start)).Msg("market metrics refresh finished")
}

func toSymbolStrings(symbols []domain.Symbol) []string {
	out := make([]string, len(symbols))
	for i, s := range symbols {
		out[i] = s.Symbol
	}
	return out
}
