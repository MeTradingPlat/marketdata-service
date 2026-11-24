package tastytrade

import (
	"context"
	"net/url"
	"time"
)

const (
	// optionQuoteChunkSize: 100 es el limite duro real de TastyTrade para
	// /market-data/by-type (igual que dividendChunkSize en dividends.go).
	optionQuoteChunkSize = 100
	optionQuoteThrottle  = 300 * time.Millisecond
)

type optionMarketDataResponse struct {
	Data struct {
		Items []struct {
			OpenInterest any `json:"open-interest"`
		} `json:"items"`
	} `json:"data"`
}

func (g *Gateway) sumOpenInterest(ctx context.Context, expiration optionExpiration) (float64, error) {
	occSymbols := strikeOptionSymbols(expiration.Strikes)
	if len(occSymbols) == 0 {
		return 0, nil
	}

	policy := chunkPolicy{
		size:     optionQuoteChunkSize,
		throttle: optionQuoteThrottle,
		attempts: optionChainAttempts,
		backoff:  optionRetryBackoff,
	}
	values, err := fetchInChunks(ctx, occSymbols, policy, g.fetchOptionQuotes)
	if err != nil {
		return 0, err
	}

	var total float64
	for _, v := range values {
		total += v
	}
	return total, nil
}

func (g *Gateway) fetchOptionQuotes(ctx context.Context, occSymbols []string) ([]float64, error) {
	q := url.Values{}
	for _, s := range occSymbols {
		q.Add("equity-option", s)
	}
	mr, err := doAuthenticatedJSON[optionMarketDataResponse](ctx, g, g.oauth.cfg.BaseURL+"/market-data/by-type?"+q.Encode())
	if err != nil {
		return nil, err
	}

	values := make([]float64, 0, len(mr.Data.Items))
	for _, item := range mr.Data.Items {
		values = append(values, openInterestValue(item.OpenInterest))
	}
	return values, nil
}

func strikeOptionSymbols(strikes []optionStrike) []string {
	symbols := make([]string, 0, len(strikes)*2)
	for _, s := range strikes {
		if s.Call != "" {
			symbols = append(symbols, s.Call)
		}
		if s.Put != "" {
			symbols = append(symbols, s.Put)
		}
	}
	return symbols
}
