package tastytrade

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

const (
	dividendChunkSize     = 250
	dividendChunkThrottle = 500 * time.Millisecond
)

type marketDataResponse struct {
	Data struct {
		Items []struct {
			Symbol            string  `json:"symbol"`
			DividendAmount    *string `json:"dividend-amount"`
			DividendFrequency *string `json:"dividend-frequency"`
			IsTradingHalted   *bool   `json:"is-trading-halted"`
			HaltStartTime     *int64  `json:"halt-start-time"`
			HaltEndTime       *int64  `json:"halt-end-time"`
		} `json:"items"`
	} `json:"data"`
}

// DividendInfo trae dividend-amount/dividend-frequency de /market-data/by-type
// en lotes -- el mismo endpoint que ya se usa para el precio actual, solo
// que leyendo dos campos mas. Ausente en la respuesta (no null, la clave no
// aparece) significa que el emisor genuinamente no reparte dividendo --
// confirmado con muestra real de los 6 mercados (SPACs, warrants, ETNs y
// ETFs apalancados sin distribucion), no es un hueco de datos.
func (g *Gateway) DividendInfo(ctx context.Context, symbols []string) ([]domain.Fundamentals, error) {
	var all []domain.Fundamentals
	for i := 0; i < len(symbols); i += dividendChunkSize {
		if i > 0 {
			time.Sleep(dividendChunkThrottle)
		}
		end := min(i+dividendChunkSize, len(symbols))
		chunk, err := g.fetchDividendChunk(ctx, symbols[i:end])
		if err != nil {
			return nil, err
		}
		all = append(all, chunk...)
	}
	return all, nil
}

func (g *Gateway) fetchDividendChunk(ctx context.Context, symbols []string) ([]domain.Fundamentals, error) {
	q := url.Values{}
	for _, s := range symbols {
		q.Add("equity", s)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.oauth.cfg.BaseURL+"/market-data/by-type?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("building market-data request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.oauth.AccessToken())

	resp, err := g.oauth.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling market-data endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("market-data endpoint returned status %d", resp.StatusCode)
	}

	var mr marketDataResponse
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return nil, fmt.Errorf("decoding market-data response: %w", err)
	}

	result := make([]domain.Fundamentals, 0, len(mr.Data.Items))
	for _, item := range mr.Data.Items {
		result = append(result, domain.Fundamentals{
			Symbol:            item.Symbol,
			DividendAmount:    parseFloatOrZero(item.DividendAmount),
			DividendFrequency: parseFloatOrZero(item.DividendFrequency),
			TradingStatus:     tradingStatusOf(item.IsTradingHalted),
			HaltStartTime:     int64Or(item.HaltStartTime, -1),
			HaltEndTime:       int64Or(item.HaltEndTime, -1),
		})
	}
	return result, nil
}

// tradingStatusOf sigue el mismo mapeo que ya probo el servicio Java contra
// is-trading-halted real -- ausente en la respuesta (nil) es distinto de
// "false" pero para nuestro caso ambos significan lo mismo: activo.
func tradingStatusOf(halted *bool) string {
	if halted != nil && *halted {
		return "HALTED"
	}
	return "ACTIVE"
}

func int64Or(v *int64, fallback int64) int64 {
	if v == nil {
		return fallback
	}
	return *v
}

func parseFloatOrZero(s *string) float64 {
	if s == nil {
		return 0
	}
	v, err := strconv.ParseFloat(*s, 64)
	if err != nil {
		return 0
	}
	return v
}
