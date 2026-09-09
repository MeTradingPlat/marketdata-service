package tastytrade

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// doAuthenticatedJSON hace un GET Bearer-autenticado contra la API REST de
// TastyTrade y decodifica el body JSON en T -- colapsa el boilerplate de
// armar request/header de auth/chequeo de status/decode repetido en
// equities.go, market_metrics.go, dividends.go, earnings_reports.go y
// option_chains.go.
func doAuthenticatedJSON[T any](ctx context.Context, g *Gateway, url string) (T, error) {
	var out T
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return out, fmt.Errorf("building request to %s: %w", url, err)
	}
	req.Header.Set("Authorization", "Bearer "+g.oauth.AccessToken())

	resp, err := g.oauth.httpClient.Do(req)
	if err != nil {
		return out, fmt.Errorf("calling %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return out, fmt.Errorf("%s returned status %d", url, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, fmt.Errorf("decoding response from %s: %w", url, err)
	}
	return out, nil
}
