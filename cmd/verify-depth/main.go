package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/external/tastytrade"
	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

// cmd/verify-depth mide cuanto tarda de verdad la rafaga historica completa
// de UN simbolo, sin el limite de historyDeepWait (90s) de por medio --
// confirmado en vivo el 2026-08-31: FXI/PFE/IBIT quedaron con M1 mucho mas
// corto que el resto del universo, cada uno cortado en una fecha distinta
// (la firma de un timeout cortando a mitad de una rafaga, no de un limite
// real del proveedor). Este programa responde la pregunta directamente: si
// se le da tiempo de sobra, ¿la rafaga sigue mandando datos pasados los
// 90s, y cuanto tarda en terminar de verdad?
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
		symbol = "FXI"
	}
	tf := domain.Timeframe(os.Getenv("TIMEFRAME"))
	if tf == "" {
		tf = domain.M1
	}
	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	if v := os.Getenv("FROM"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			from = t
		}
	}
	wait := 10 * time.Minute
	if v := os.Getenv("WAIT_SECONDS"); v != "" {
		if s, err := strconv.Atoi(v); err == nil {
			wait = time.Duration(s) * time.Second
		}
	}

	fmt.Printf("fetching %s %s since %s, wait budget %s...\n", symbol, tf, from.Format("2006-01-02"), wait)
	start := time.Now()
	candles, err := pool.FetchHistoryWithWait(ctx, symbol, tf, from, wait)
	elapsed := time.Since(start)
	if err != nil {
		fmt.Printf("fetch error after %s: %v\n", elapsed, err)
		os.Exit(1)
	}
	fmt.Printf("done in %s -- %d candles\n", elapsed, len(candles))
	if len(candles) > 0 {
		fmt.Printf("oldest: %s\n", candles[0].Timestamp.UTC().Format("2006-01-02 15:04"))
		fmt.Printf("newest: %s\n", candles[len(candles)-1].Timestamp.UTC().Format("2006-01-02 15:04"))
	}
}
