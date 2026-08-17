package main

import (
	"context"
	"fmt"
	"os"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/external/tastytrade"
	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/repository/timescale"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs/storage"
)

func main() {
	cfg := configs.Load()
	ctx := context.Background()

	oauth := tastytrade.NewOAuth(tastytrade.OAuthConfig{
		BaseURL:      cfg.TastyTradeBaseURL,
		ClientID:     cfg.TastyTradeClientID,
		ClientSecret: cfg.TastyTradeClientSecret,
		RefreshToken: cfg.TastyTradeRefreshToken,
	})
	if _, err := oauth.RefreshAccessToken(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "oauth refresh failed:", err)
		os.Exit(1)
	}

	gateway := tastytrade.NewGateway(oauth, nil)

	symbol := os.Getenv("SYMBOL")
	if symbol == "" {
		symbol = "MDXH"
	}

	reports, err := gateway.EarningsReports(ctx, symbol)
	if err != nil {
		fmt.Fprintln(os.Stderr, "earnings reports failed:", err)
		os.Exit(1)
	}
	fmt.Printf("=== %s: %d reportes con EPS ===\n", symbol, len(reports))
	occurred, predicted := domain.LastEarningsReport(reports)
	fmt.Printf("occurredDate: %s\npredictedNext: %s\n", occurred, predicted)

	if os.Getenv("WRITE_DB") != "1" {
		return
	}

	pool := storage.ConnInstanceTimescale(cfg)
	defer pool.Close()
	repo := timescale.NewFundamentalsRepository(pool)
	update := domain.Fundamentals{Symbol: symbol, OccurredDate: occurred}
	if predicted != "" {
		update.NextEarningsDate = predicted
	}
	if err := repo.UpsertEarningsHistory(ctx, []domain.Fundamentals{update}); err != nil {
		fmt.Fprintln(os.Stderr, "upsert failed:", err)
		os.Exit(1)
	}
	fmt.Println("DB upsert OK")
}
