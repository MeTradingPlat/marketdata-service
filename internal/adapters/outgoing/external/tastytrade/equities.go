package tastytrade

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

const (
	equitiesPerPage  = 1000
	equitiesMaxPages = 100
)

type equitiesResponse struct {
	Data struct {
		Items []struct {
			Symbol       string `json:"symbol"`
			Description  string `json:"description"`
			ListedMarket string `json:"listed-market"`
		} `json:"items"`
	} `json:"data"`
}

func (g *Gateway) ActiveSymbols(ctx context.Context) ([]domain.Symbol, error) {
	var all []domain.Symbol
	for page := 0; page < equitiesMaxPages; page++ {
		batch, err := g.fetchEquitiesPage(ctx, page, equitiesPerPage)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		all = append(all, batch...)
		if len(batch) < equitiesPerPage {
			break
		}
	}
	return all, nil
}

func (g *Gateway) fetchEquitiesPage(ctx context.Context, page, perPage int) ([]domain.Symbol, error) {
	url := fmt.Sprintf("%s/instruments/equities/active?per-page=%d&page-offset=%d", g.oauth.cfg.BaseURL, perPage, page)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building equities request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.oauth.AccessToken())

	resp, err := g.oauth.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling equities endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("equities endpoint returned status %d", resp.StatusCode)
	}

	var er equitiesResponse
	if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
		return nil, fmt.Errorf("decoding equities response: %w", err)
	}

	symbols := make([]domain.Symbol, 0, len(er.Data.Items))
	for _, item := range er.Data.Items {
		symbols = append(symbols, domain.Symbol{Symbol: item.Symbol, Description: item.Description, Market: item.ListedMarket})
	}
	return symbols, nil
}
