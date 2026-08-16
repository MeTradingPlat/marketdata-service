package finra

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
)

const finraURLTemplate = "https://cdn.finra.org/equity/otcmarket/biweekly/shrt%s.csv"
const candidateCount = 6

var httpClient = &http.Client{Timeout: 30 * time.Second}

// Client no tiene estado propio (FINRA no requiere autenticacion ni cache
// en disco) -- existe solo para que este paquete implemente
// out.ShortInterestGateway como metodo, en vez de una funcion suelta.
type Client struct{}

func NewClient() *Client {
	return &Client{}
}

// DownloadLatest prueba varias fechas de settlement candidatas (FINRA
// publica quincenal) hasta que alguna responda 200 -- no hay forma de saber
// de antemano cual es la mas reciente ya publicada. Sin API key.
func (c *Client) DownloadLatest(ctx context.Context) map[string]domain.ShortInterestRecord {
	for _, candidate := range recentSettlementDates(candidateCount) {
		if result := tryDownload(ctx, candidate); result != nil {
			return result
		}
	}
	return map[string]domain.ShortInterestRecord{}
}

func tryDownload(ctx context.Context, candidate time.Time) map[string]domain.ShortInterestRecord {
	url := fmt.Sprintf(finraURLTemplate, candidate.Format("20060102"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	return parseFinraCsv(body)
}
