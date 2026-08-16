package catchup

import (
	"context"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/ports/out"
	"github.com/rs/zerolog/log"
)

// topSymbolsPerMarket acota el refresco de fundamentales a un piloto de 10
// simbolos por mercado (los de mas volumen real, ver TopSymbolsPerMarket)
// en vez de todo el universo rastreado -- REST de sobra, sin presionar el
// rate limit de TastyTrade mientras se valida que los datos vienen bien.
const topSymbolsPerMarket = 10

// RefreshFundamentals trae dividendos/halt (DividendInfo, /market-data/by-type)
// y market-cap/beta/liquidez/IV/earnings (MarketMetrics, /market-metrics)
// para el universo acotado -- dos llamadas REST separadas, cada una
// UPSERTea solo sus propias columnas (ver FundamentalsRepository), asi que
// una puede fallar sin pisar lo que la otra ya guardo. Corre despues de
// D1/H1 en el barrido nocturno porque es puro REST/BD, no compite por
// conexiones DxLink como las fases de velas.
func RefreshFundamentals(ctx context.Context, gateway out.MarketDataGateway, symbolsRepo out.SymbolRepository, fundamentalsRepo out.FundamentalsRepository) {
	tracked, err := symbolsRepo.TopSymbolsPerMarket(ctx, topSymbolsPerMarket)
	if err != nil {
		log.Error().Err(err).Msg("fundamentals refresh: failed to list top symbols per market")
		return
	}
	symbols := make([]string, len(tracked))
	for i, s := range tracked {
		symbols[i] = s.Symbol
	}

	start := time.Now()

	dividends, err := gateway.DividendInfo(ctx, symbols)
	if err != nil {
		log.Error().Err(err).Msg("dividend/halt refresh failed")
	} else if err := fundamentalsRepo.UpsertDividends(ctx, dividends); err != nil {
		log.Error().Err(err).Msg("upserting dividend/halt refresh failed")
	}

	metrics, err := gateway.MarketMetrics(ctx, symbols)
	if err != nil {
		log.Error().Err(err).Msg("market metrics refresh failed")
	} else if err := fundamentalsRepo.UpsertMarketMetrics(ctx, metrics); err != nil {
		log.Error().Err(err).Msg("upserting market metrics refresh failed")
	}

	log.Info().Int("symbols", len(symbols)).Dur("elapsed", time.Since(start)).Msg("fundamentals refresh finished")
}
