package finra

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/rs/zerolog/log"
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
// publica quincenal con ~2 semanas de lag: el archivo de la quincena en
// curso aun no existe, los viejos se purgan del CDN) hasta que alguna
// responda 200. Devuelve error cuando NINGUNA responde -- confirmado en
// vivo: si todo falla en silencio (como el refresh de las 16:03 del
// 2026-08-18) el paso se registra como done con cero datos y el short
// interest se pierde hasta la ventana siguiente.
func (c *Client) DownloadLatest(ctx context.Context) (map[string]domain.ShortInterestRecord, error) {
	for _, candidate := range recentSettlementDates(candidateCount) {
		result, err := tryDownload(ctx, candidate)
		if err != nil {
			log.Warn().Str("candidate", candidate.Format("2006-01-02")).Err(err).Msg("finra download attempt failed")
			continue
		}
		if len(result) == 0 {
			log.Warn().Str("file", candidate.Format("20060102")).Msg("finra csv downloaded but parsed empty, suspicious")
		}
		return result, nil
	}
	return map[string]domain.ShortInterestRecord{}, fmt.Errorf("no finra settlement file responded in the last %d candidates", candidateCount)
}

func tryDownload(ctx context.Context, candidate time.Time) (map[string]domain.ShortInterestRecord, error) {
	url := fmt.Sprintf(finraURLTemplate, candidate.Format("20060102"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request for %s: %w", url, err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d for %s", resp.StatusCode, url)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", url, err)
	}
	return parseFinraCsv(body), nil
}
