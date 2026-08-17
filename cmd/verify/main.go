package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/external/tastytrade"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs"
)

// verify abre su PROPIA conexion DxLink, independiente de la del servicio
// en marcha, y pide en vivo los ultimos N minutos de M1 de un simbolo --
// para comparar contra lo que el servicio ya guardo via merge de ticks en
// vivo. Al ser una sesion nueva, no comparte canal/conexion con el pool del
// servicio real: no hay ningun riesgo de colision de suscripcion.
func main() {
	symbol := flag.String("symbol", "AAPL", "symbol to verify")
	minutes := flag.Int("minutes", 10, "how many minutes back to fetch")
	flag.Parse()

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

	qt := tastytrade.NewQuoteToken(oauth)
	if err := qt.Refresh(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "quote token refresh failed:", err)
		os.Exit(1)
	}

	urlFunc := qt.DxlinkURL
	if cfg.DxlinkURLOverride != "" {
		urlFunc = func() string { return cfg.DxlinkURLOverride }
	}
	connFactory := func(ctx context.Context) (*tastytrade.DxLinkConn, error) {
		conn := tastytrade.NewDxLinkConn(urlFunc, qt.Token)
		if err := conn.Connect(ctx); err != nil {
			return nil, err
		}
		return conn, nil
	}
	pool := tastytrade.NewCandlePool(connFactory, 1)

	from := time.Now().Add(-time.Duration(*minutes) * time.Minute)
	candles, err := pool.FetchHistory(ctx, *symbol, domain.M1, from)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fetch history failed:", err)
		os.Exit(1)
	}

	enc := json.NewEncoder(os.Stdout)
	for _, c := range candles {
		_ = enc.Encode(c)
	}
}
