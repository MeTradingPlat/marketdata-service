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
// option_chains.go. Ante un 401 renueva el token (una sola vez, coalescido
// entre llamadas concurrentes) y reintenta una vez.
func doAuthenticatedJSON[T any](ctx context.Context, g *Gateway, url string) (T, error) {
	token := g.oauth.AccessToken()
	out, status, err := getJSONOnce[T](ctx, g, url, token)
	if status != http.StatusUnauthorized {
		return out, err
	}
	if refreshErr := g.oauth.RefreshIfStale(ctx, token); refreshErr != nil {
		return out, err
	}
	out, _, err = getJSONOnce[T](ctx, g, url, g.oauth.AccessToken())
	return out, err
}

func getJSONOnce[T any](ctx context.Context, g *Gateway, url, token string) (T, int, error) {
	var out T
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return out, 0, fmt.Errorf("building request to %s: %w", url, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := g.oauth.httpClient.Do(req)
	if err != nil {
		return out, 0, fmt.Errorf("calling %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return out, resp.StatusCode, &httpStatusError{url: url, status: resp.StatusCode}
	}

	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, resp.StatusCode, fmt.Errorf("decoding response from %s: %w", url, err)
	}
	return out, resp.StatusCode, nil
}
