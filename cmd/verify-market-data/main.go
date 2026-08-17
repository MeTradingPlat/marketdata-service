package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/MeTradingPlat/marketdata-service/internal/adapters/outgoing/external/tastytrade"
	"github.com/MeTradingPlat/marketdata-service/internal/infrastructure/configs"
)

// verify-market-data pide /market-data/by-type para un puñado de simbolos
// reales y vuelca la respuesta cruda -- mismo proposito que verify-metrics
// pero para el endpoint que trae OHLC/halt/trading-status, para decidir con
// datos reales si REST alcanza en vez de sumar otro pool de conexiones
// DxLink dedicado a fundamentales.
func main() {
	cfg := configs.Load()
	ctx := context.Background()

	oauth := tastytrade.NewOAuth(tastytrade.OAuthConfig{
		BaseURL:      cfg.TastyTradeBaseURL,
		ClientID:     cfg.TastyTradeClientID,
		ClientSecret: cfg.TastyTradeClientSecret,
		RefreshToken: cfg.TastyTradeRefreshToken,
	})
	token, err := oauth.RefreshAccessToken(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "oauth refresh failed:", err)
		os.Exit(1)
	}

	symbols := []string{"AAPL", "SPY", "GOOGL", "TSLA", "IBM"}
	url := fmt.Sprintf("%s/market-data/by-type?equity=%s", cfg.TastyTradeBaseURL, strings.Join(symbols, ","))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "building request failed:", err)
		os.Exit(1)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "request failed:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		fmt.Fprintln(os.Stderr, "decode failed:", err)
		os.Exit(1)
	}

	pretty, _ := json.MarshalIndent(body, "", "  ")
	fmt.Println("status:", resp.StatusCode)
	fmt.Println(string(pretty))
}
