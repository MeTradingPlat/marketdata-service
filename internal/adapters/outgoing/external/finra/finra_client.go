package finra

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/core/domain"
	"github.com/MeTradingPlat/marketdata-service/internal/pkg/httpretry"
	"github.com/rs/zerolog/log"
)

const finraURLTemplate = "https://cdn.finra.org/equity/otcmarket/biweekly/shrt%s.csv"
const candidateCount = 6

var httpClient = &http.Client{Timeout: 30 * time.Second}

var finraRetryDelays = []time.Duration{2 * time.Second, 5 * time.Second, 15 * time.Second}

type Client struct {
	urlTemplate string
	delays      []time.Duration
	candidates  func(count int) []time.Time
}

func NewClient() *Client {
	return &Client{urlTemplate: finraURLTemplate, delays: finraRetryDelays, candidates: recentSettlementDates}
}

// DownloadLatest prueba varias fechas de settlement candidatas (FINRA
// publica quincenal con ~2 semanas de lag: el archivo de la quincena en
// curso aun no existe, los viejos se purgan del CDN) hasta que alguna
// responda 200. El CDN responde 403 (no 404) a un archivo que no existe,
// verificado el 2026-11-18. Solo 403/404 significan "aun no publicado, probar
// el anterior": un fallo de red, 429 o 5xx se reintenta y, si persiste, devuelve
// error en vez de caer en silencio a un archivo mas viejo con datos
// desactualizados. Devuelve error cuando NINGUNA candidata esta publicada --
// confirmado en vivo: si todo falla en silencio (como el refresh de las 16:03
// del 2026-08-18) el paso se registra como done con cero datos y el short
// interest se pierde hasta la ventana siguiente.
func (c *Client) DownloadLatest(ctx context.Context) (map[string]domain.ShortInterestRecord, error) {
	for _, candidate := range c.candidates(candidateCount) {
		result, err := c.tryDownload(ctx, candidate)
		if err == nil {
			if len(result) == 0 {
				log.Warn().Str("file", candidate.Format("20060102")).Msg("finra csv downloaded but parsed empty, suspicious")
			}
			return result, nil
		}
		if !notPublished(err) {
			return map[string]domain.ShortInterestRecord{}, fmt.Errorf("downloading finra settlement file %s: %w", candidate.Format("20060102"), err)
		}
		log.Warn().Str("candidate", candidate.Format("2006-01-02")).Err(err).Msg("finra settlement file not published, trying the previous one")
	}
	return map[string]domain.ShortInterestRecord{}, fmt.Errorf("no finra settlement file published in the last %d candidates", candidateCount)
}

func (c *Client) tryDownload(ctx context.Context, candidate time.Time) (map[string]domain.ShortInterestRecord, error) {
	url := fmt.Sprintf(c.urlTemplate, candidate.Format("20060102"))
	resp, err := httpretry.Get(ctx, httpClient, url, httpretry.Options{Delays: c.delays})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", url, err)
	}
	return parseFinraCsv(body), nil
}

func notPublished(err error) bool {
	code := httpretry.StatusCode(err)
	return code == http.StatusForbidden || code == http.StatusNotFound
}
