package secedgar

import (
	"context"
	"strings"
)

const submissionsURLTemplate = "https://data.sec.gov/submissions/CIK%010d.json"
const beneficialOwnerDocURLTemplate = "https://www.sec.gov/Archives/edgar/data/%d/%s/%s"

// relevantFormPrefixes: SOLO 13D (bloques activistas/estrategicos). 13G
// quedan afuera a proposito: los filers 13G son en su mayoria fondos
// pasivos (Vanguard/BlackRock) cuyas posiciones SI forman parte del float
// real -- restarlos dejaba floats sistematicamente bajos (confirmado contra
// datos reales: AAPL -7%, UEC -43%, CBOE -26% vs el float publicado). El
// prefijo "SC 13D" cubre tambien las enmiendas "SC 13D/A".
var relevantFormPrefixes = []string{"SC 13D", "SCHEDULE 13D"}

// BeneficialOwnersClient resuelve bloques estrategicos 5%+ (Schedule 13D
// unicamente -- ver relevantFormPrefixes) -- a diferencia de los otros
// clientes de este paquete, no hay archivo bulk: se pide por simbolo bajo
// demanda. Solo cubre filings posteriores al mandato de XML estructurado
// de diciembre 2024; los anteriores se omiten a proposito en vez de
// depender de scraping de HTML.
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
