package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/external/tastytrade"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs"
	"github.com/rs/zerolog"
)

// verify-stop prueba StopAllLive de forma aislada: suscribe un par de
// simbolos en vivo, mide cuanto tarda en desuscribir todo, y vuelve a
// suscribir para confirmar que el ciclo completo (como lo hara la ventana
// de mantenimiento) funciona de punta a punta -- con su propia conexion,
// sin tocar el pool del servicio en produccion.
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
	pool := tastytrade.NewCandlePool(connFactory, 5)

	symbols := []string{"AAPL", "MSFT", "GOOGL"}
	for _, s := range symbols {
		sym := s
		onClosed := func(c domain.Candle) {
			fmt.Printf("[closed candle] %s %v O=%.2f H=%.2f L=%.2f C=%.2f\n", sym, c.Timestamp, c.Open, c.High, c.Low, c.Close)
		}
		if err := pool.SubscribeLive(ctx, sym, time.Time{}, onClosed, nil); err != nil {
			fmt.Fprintln(os.Stderr, "subscribe failed for", sym, ":", err)
			os.Exit(1)
		}
		fmt.Println("subscribed live:", sym)
	}

	fmt.Println("waiting 15s to receive some live ticks...")
	time.Sleep(15 * time.Second)

	fmt.Println("calling StopAllLive...")
	start := time.Now()
	pool.StopAllLive(ctx)
	fmt.Printf("StopAllLive returned after %v\n", time.Since(start))

	fmt.Println("waiting 10s before resubscribing (simulated maintenance gap)...")
	time.Sleep(10 * time.Second)

	from := time.Now().Add(-5 * time.Minute)
	for _, s := range symbols {
		sym := s
		onClosed := func(c domain.Candle) {
			fmt.Printf("[resubscribed closed candle] %s %v O=%.2f H=%.2f L=%.2f C=%.2f\n", sym, c.Timestamp, c.Open, c.High, c.Low, c.Close)
		}
		start := time.Now()
		if err := pool.SubscribeLive(ctx, sym, from, onClosed, nil); err != nil {
			fmt.Fprintln(os.Stderr, "resubscribe failed for", sym, ":", err)
			os.Exit(1)
		}
		fmt.Printf("resubscribed live: %s (took %v)\n", sym, time.Since(start))
	}

	fmt.Println("waiting 20s to confirm streaming resumed cleanly...")
	time.Sleep(20 * time.Second)
	fmt.Println("done")
}
