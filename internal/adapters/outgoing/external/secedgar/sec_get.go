package secedgar

import (
	"context"
	"net/http"
	"time"

	"github.com/MeTradingPlat/marketdata-service/internal/pkg/httpretry"
	"golang.org/x/time/rate"
)

const (
	secRequestsPerSecond = 8
	secRequestBurst      = 2
)

var (
	secLimiter     = rate.NewLimiter(secRequestsPerSecond, secRequestBurst)
	secRetryDelays = []time.Duration{2 * time.Second, 5 * time.Second, 15 * time.Second}
)

func secGet(ctx context.Context, url string, extra http.Header) (*http.Response, error) {
	header := http.Header{"User-Agent": {UserAgent}}
	for name, values := range extra {
		header[name] = values
	}
	return httpretry.Get(ctx, httpClient, url, httpretry.Options{Delays: secRetryDelays, Header: header, Wait: secLimiter.Wait})
}
