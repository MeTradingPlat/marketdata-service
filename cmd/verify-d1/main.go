package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/external/tastytrade"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

func main() {
	ctx := context.Background()
	oauth := tastytrade.NewOAuth(tastytrade.OAuthConfig{
		BaseURL:      "https://api.tastyworks.com",
		ClientID:     os.Getenv("TT_CLIENT_ID"),
		ClientSecret: os.Getenv("TT_CLIENT_SECRET"),
		RefreshToken: os.Getenv("TT_REFRESH_TOKEN"),
	})
	qt := tastytrade.NewQuoteToken(oauth)
	if err := qt.Refresh(ctx); err != nil {
		fmt.Println("quote token:", err)
		os.Exit(1)
	}
	connFactory := func(ctx context.Context) (*tastytrade.DxLinkConn, error) {
		conn := tastytrade.NewDxLinkConn(qt.DxlinkURL, qt.Token)
		if err := conn.Connect(ctx); err != nil {
			return nil, err
		}
		return conn, nil
	}
	pool := tastytrade.NewCandlePool(connFactory, 1)
	if err := pool.WarmUp(ctx); err != nil {
		fmt.Println("warmup:", err)
		os.Exit(1)
	}

	symbol := os.Getenv("SYMBOL")
	if symbol == "" {
		symbol = "NVDA"
	}
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	if v := os.Getenv("FROM"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			from = t
		}
	}

	for _, tf := range []domain.Timeframe{domain.D1, domain.H1, domain.M1} {
		candles, err := pool.FetchHistory(ctx, symbol, tf, from)
		if err != nil {
			fmt.Printf("%s fetch error: %v\n", tf, err)
			continue
		}
		fmt.Printf("%s candles since %s: %d\n", tf, from.Format("2006-01-02"), len(candles))
		for _, c := range candles {
			fmt.Printf("  %s  o=%.4f c=%.4f v=%d\n",
				c.Timestamp.UTC().Format("2006-01-02 15:04"), c.Open, c.Close, c.Volume)
		}
	}
}
