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

// verify-metrics pide /market-metrics para un puñado de simbolos reales y
// vuelca la respuesta cruda -- para ver campo por campo que llega
// poblado y que no, antes de construir el modelo de fundamentales sobre
// una suposicion.
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
	url := fmt.Sprintf("%s/market-metrics?symbols=%s", cfg.TastyTradeBaseURL, strings.Join(symbols, ","))
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
