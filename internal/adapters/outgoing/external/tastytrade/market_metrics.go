package tastytrade

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

const (
	marketMetricsChunkSize     = 250
	marketMetricsChunkThrottle = 500 * time.Millisecond
)

type marketMetricsResponse struct {
	Data struct {
		Items []struct {
			Symbol                      string   `json:"symbol"`
			Beta                        *string  `json:"beta"`
			MarketCap                   *float64 `json:"market-cap"`
			EarningsPerShare            *string  `json:"earnings-per-share"`
			Lendability                 *string  `json:"lendability"`
			BorrowRate                  *string  `json:"borrow-rate"`
			LiquidityValue              *string  `json:"liquidity-value"`
			LiquidityRating             *int     `json:"liquidity-rating"`
			ImpliedVolatilityIndex      *string  `json:"implied-volatility-index"`
			ImpliedVolatilityIndexRank  *string  `json:"implied-volatility-index-rank"`
			ImpliedVolatilityPercentile *string  `json:"implied-volatility-percentile"`
			Earnings                    *struct {
				ExpectedReportDate *string `json:"expected-report-date"`
			} `json:"earnings"`
		} `json:"items"`
	} `json:"data"`
}

// MarketMetrics trae market-cap/beta/liquidez/IV/proximo earnings de
// /market-metrics -- confirmado campo por campo contra respuestas reales
// (cmd/verify-metrics) antes de escribir este parseo. market-cap y
// liquidity-rating llegan como numero JSON crudo, el resto como string
// (asi los devuelve TastyTrade, no es inconsistencia nuestra).
func (g *Gateway) MarketMetrics(ctx context.Context, symbols []string) ([]domain.Fundamentals, error) {
	var all []domain.Fundamentals
	for i := 0; i < len(symbols); i += marketMetricsChunkSize {
		if i > 0 {
			time.Sleep(marketMetricsChunkThrottle)
		}
		end := min(i+marketMetricsChunkSize, len(symbols))
		chunk, err := g.fetchMarketMetricsChunk(ctx, symbols[i:end])
		if err != nil {
			return nil, err
		}
		all = append(all, chunk...)
	}
	return all, nil
}

func (g *Gateway) fetchMarketMetricsChunk(ctx context.Context, symbols []string) ([]domain.Fundamentals, error) {
	q := url.Values{"symbols": {strings.Join(symbols, ",")}}
	mr, err := doAuthenticatedJSON[marketMetricsResponse](ctx, g, g.oauth.cfg.BaseURL+"/market-metrics?"+q.Encode())
	if err != nil {
		return nil, err
	}

	result := make([]domain.Fundamentals, 0, len(mr.Data.Items))
	for _, item := range mr.Data.Items {
		f := domain.Fundamentals{
			Symbol:                      item.Symbol,
			Beta:                        parseFloatOrZero(item.Beta),
			Eps:                         parseFloatOrZero(item.EarningsPerShare),
			BorrowRate:                  parseFloatOrZero(item.BorrowRate),
			Liquidity:                   parseFloatOrZero(item.LiquidityValue),
			ImpliedVolatilityIndex:      parseFloatOrZero(item.ImpliedVolatilityIndex),
			ImpliedVolatilityRank:       parseFloatOrZero(item.ImpliedVolatilityIndexRank),
			ImpliedVolatilityPercentile: parseFloatOrZero(item.ImpliedVolatilityPercentile),
		}
		if item.MarketCap != nil {
			f.MarketCap = *item.MarketCap
		}
		if item.LiquidityRating != nil {
			f.LiquidityRating = *item.LiquidityRating
		}
		if item.Lendability != nil {
			f.Lendability = *item.Lendability
		}
		if item.Earnings != nil && item.Earnings.ExpectedReportDate != nil {
			f.NextEarningsDate = *item.Earnings.ExpectedReportDate
		}
		result = append(result, f)
	}
	return result, nil
}
