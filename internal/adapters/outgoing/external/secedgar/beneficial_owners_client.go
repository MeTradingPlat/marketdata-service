package secedgar

import (
	"context"
	"strings"
)

const submissionsURLTemplate = "https://data.sec.gov/submissions/CIK%010d.json"
const beneficialOwnerDocURLTemplate = "https://www.sec.gov/Archives/edgar/data/%d/%s/%s"

var relevantFormPrefixes = []string{"SC 13D", "SC 13G", "SCHEDULE 13D", "SCHEDULE 13G"}

// BeneficialOwnersClient resuelve holders institucionales de 5%+ (Schedule
// 13D/13G) -- a diferencia de los otros clientes de este paquete, no hay
// archivo bulk: se pide por simbolo bajo demanda. Solo cubre filings
// posteriores al mandato de XML estructurado de diciembre 2024; los
// anteriores se omiten a proposito en vez de depender de scraping de HTML.
type BeneficialOwnersClient struct {
	tickerCik *TickerCikLookup
}

func NewBeneficialOwnersClient(tickerCik *TickerCikLookup) *BeneficialOwnersClient {
	return &BeneficialOwnersClient{tickerCik: tickerCik}
}

func (c *BeneficialOwnersClient) FetchBeneficialOwnerShares(ctx context.Context, symbol string, excludeCiks map[string]bool) int64 {
	issuerCik, ok := c.tickerCik.TickerToCikMap(ctx)[strings.ToUpper(symbol)]
	if !ok {
		return 0
	}

	candidates, err := findCandidateFilings(ctx, issuerCik)
	if err != nil {
		return 0
	}
	return sumLatestPerFiler(ctx, issuerCik, candidates, excludeCiks)
}
