package tastytrade

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

const earningsHistoryLookback = -2 // años

type earningsReportsResponse struct {
	Data struct {
		Items []struct {
			OccurredDate *string  `json:"occurred-date"`
			Eps          *float64 `json:"eps"`
		} `json:"items"`
	} `json:"data"`
}

// EarningsReports trae hasta 2 años de reportes pasados de un simbolo --
// GET /market-metrics/historic-corporate-events/earnings-reports/{symbol}.
// A diferencia de /market-metrics (batch de hasta 250 simbolos), este
// endpoint es por-simbolo, sin forma de pedir varios de una sola vez.
func (g *Gateway) EarningsReports(ctx context.Context, symbol string) ([]domain.EarningsReportItem, error) {
	startDate := time.Now().AddDate(earningsHistoryLookback, 0, 0).Format("2006-01-02")
	url := fmt.Sprintf("%s/market-metrics/historic-corporate-events/earnings-reports/%s?start-date=%s",
		g.oauth.cfg.BaseURL, symbol, startDate)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building earnings reports request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+g.oauth.AccessToken())

	resp, err := g.oauth.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling earnings reports endpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("earnings reports endpoint returned status %d", resp.StatusCode)
	}

	var er earningsReportsResponse
	if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
		return nil, fmt.Errorf("decoding earnings reports response: %w", err)
	}

	result := make([]domain.EarningsReportItem, 0, len(er.Data.Items))
	for _, item := range er.Data.Items {
		if item.OccurredDate == nil || item.Eps == nil {
			continue
		}
		result = append(result, domain.EarningsReportItem{OccurredDate: *item.OccurredDate, Eps: *item.Eps})
	}
	return result, nil
}
