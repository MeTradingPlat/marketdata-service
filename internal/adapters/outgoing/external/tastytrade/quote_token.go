package tastytrade

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

type QuoteToken struct {
	oauth *OAuth

	mu        sync.RWMutex
	token     string
	dxlinkURL string
}

func NewQuoteToken(oauth *OAuth) *QuoteToken {
	return &QuoteToken{oauth: oauth}
}

func (q *QuoteToken) Token() string {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.token
}

func (q *QuoteToken) DxlinkURL() string {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.dxlinkURL
}

type quoteTokenResponse struct {
	Data struct {
		Token     string `json:"token"`
		DxlinkURL string `json:"dxlink-url"`
	} `json:"data"`
}

// Refresh centraliza el reintento de un solo golpe tras 401 -- en el
// servicio Java esta logica estaba repetida por metodo, aca vive en un
// solo lugar.
func (q *QuoteToken) Refresh(ctx context.Context) error {
	if q.oauth.AccessToken() == "" {
		if _, err := q.oauth.RefreshAccessToken(ctx); err != nil {
			return err
		}
	}

	resp, status, err := q.fetch(ctx)
	if err != nil {
		return err
	}
	if status == http.StatusUnauthorized {
		if _, err := q.oauth.RefreshAccessToken(ctx); err != nil {
			return err
		}
		if resp, status, err = q.fetch(ctx); err != nil {
			return err
		}
	}
	if status != http.StatusOK {
		return fmt.Errorf("api-quote-tokens returned status %d", status)
	}

	q.mu.Lock()
	q.token = resp.Data.Token
	q.dxlinkURL = resp.Data.DxlinkURL
	q.mu.Unlock()
	return nil
}

func (q *QuoteToken) fetch(ctx context.Context) (quoteTokenResponse, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, q.oauth.cfg.BaseURL+"/api-quote-tokens", nil)
	if err != nil {
		return quoteTokenResponse{}, 0, fmt.Errorf("building api-quote-tokens request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+q.oauth.AccessToken())

	resp, err := q.oauth.httpClient.Do(req)
	if err != nil {
		return quoteTokenResponse{}, 0, fmt.Errorf("calling api-quote-tokens: %w", err)
	}
	defer resp.Body.Close()

	var qtr quoteTokenResponse
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&qtr); err != nil {
			return quoteTokenResponse{}, resp.StatusCode, fmt.Errorf("decoding api-quote-tokens response: %w", err)
		}
	}
	return qtr, resp.StatusCode, nil
}
