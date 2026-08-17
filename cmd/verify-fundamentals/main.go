package main

import (
	"context"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/external/tastytrade"
	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/repository/timescale"
	"github.com/MeTradingPlat/marketdata-service/internal/core/service/catchup"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs/storage"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// verify-fundamentals dispara RefreshFundamentals una sola vez a mano --
// mismo codigo real que corre en el ciclo nocturno, para no esperar a que
// termine el barrido D1/H1/M1 completo antes de ver datos reales.
func main() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	cfg := configs.Load()
	ctx := context.Background()

	oauth := tastytrade.NewOAuth(tastytrade.OAuthConfig{
		BaseURL:      cfg.TastyTradeBaseURL,
		ClientID:     cfg.TastyTradeClientID,
		ClientSecret: cfg.TastyTradeClientSecret,
		RefreshToken: cfg.TastyTradeRefreshToken,
	})
	if _, err := oauth.RefreshAccessToken(ctx); err != nil {
		log.Fatal().Err(err).Msg("oauth refresh failed")
	}
	gateway := tastytrade.NewGateway(oauth, nil)

	dbPool := storage.ConnInstanceTimescale(cfg)
	symbolsRepo := timescale.NewSymbolRepository(dbPool)
	fundamentalsRepo := timescale.NewFundamentalsRepository(dbPool)

	catchup.RefreshTradingStatus(ctx, gateway, symbolsRepo, fundamentalsRepo)
	catchup.RefreshMarketMetrics(ctx, gateway, symbolsRepo, fundamentalsRepo)
}
