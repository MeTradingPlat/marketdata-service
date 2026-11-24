package tastytrade

import (
	"context"
	"strings"
	"time"
)

const (
	optionChainAttempts = 3
	optionRetryBackoff  = time.Second
)

type nestedOptionChainResponse struct {
	Data struct {
		Items []optionChainItem `json:"items"`
	} `json:"data"`
}

type optionChainItem struct {
	OptionChainType string             `json:"option-chain-type"`
	Expirations     []optionExpiration `json:"expirations"`
}

type optionExpiration struct {
	ExpirationDate string         `json:"expiration-date"`
	ExpirationType string         `json:"expiration-type"`
	Strikes        []optionStrike `json:"strikes"`
}

type optionStrike struct {
	Call string `json:"call"`
	Put  string `json:"put"`
}

// optionExpirations consulta /option-chains/{symbol}/nested con reintentos --
// el endpoint nested devuelve todos los vencimientos con su tipo y simbolos OCC
// de call/put en una sola respuesta.
func (g *Gateway) optionExpirations(ctx context.Context, symbol string) ([]optionExpiration, error) {
	policy := chunkPolicy{size: 1, attempts: optionChainAttempts, backoff: optionRetryBackoff}
	return fetchWithRetry(ctx, []string{symbol}, policy, g.fetchOptionChain)
}

func (g *Gateway) fetchOptionChain(ctx context.Context, symbols []string) ([]optionExpiration, error) {
	cr, err := doAuthenticatedJSON[nestedOptionChainResponse](ctx, g, g.oauth.cfg.BaseURL+"/option-chains/"+symbols[0]+"/nested")
	if err != nil {
		return nil, err
	}

	// Preferir el item Standard evita mezclar contratos Mini o NonStandard
	// en la seleccion del vencimiento mensual.
	var standard, all []optionExpiration
	hasStandard := false
	for _, item := range cr.Data.Items {
		if strings.EqualFold(item.OptionChainType, "Standard") {
			hasStandard = true
			standard = append(standard, item.Expirations...)
			continue
		}
		all = append(all, item.Expirations...)
	}
	if hasStandard {
		return standard, nil
	}
	return all, nil
}
