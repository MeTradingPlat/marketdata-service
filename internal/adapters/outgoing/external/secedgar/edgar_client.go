package secedgar

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
)

const bulkFactsURL = "https://www.sec.gov/Archives/edgar/daily-index/xbrl/companyfacts.zip"

// EdgarClient resuelve sharesOutstanding contra el bulk companyfacts.zip de
// la SEC (~1.5GB), cacheado en disco una vez por dia y leido entrada por
// entrada -- la memoria pico queda acotada al JSON de una sola empresa, no
// al archivo completo (ver parseCompanyFactsZip).
type EdgarClient struct {
	tickerCik *TickerCikLookup
	cacheDir  string
}

func NewEdgarClient(tickerCik *TickerCikLookup, cacheDir string) *EdgarClient {
	return &EdgarClient{tickerCik: tickerCik, cacheDir: cacheDir}
}

func (c *EdgarClient) FetchSharesOutstanding(ctx context.Context, symbols []string) map[string]int64 {
	shares, _ := c.FetchCompanyFacts(ctx, symbols)
	return shares
}

// FetchCompanyFacts trae sharesOutstanding y la fecha del ultimo filing
// (10-Q/10-K) en UN solo pase sobre el bulk -- usar esto en vez de llamar
// FetchSharesOutstanding aparte cuando el caller necesita ambos datos, para
// no decodificar el ZIP completo (~1.5GB) dos veces en la misma corrida.
func (c *EdgarClient) FetchCompanyFacts(ctx context.Context, symbols []string) (shares map[string]int64, filingDates map[string]string) {
	cikToSymbols := make(map[string][]string)
	tickerToCik := c.tickerCik.TickerToCikMap(ctx)
	for _, symbol := range symbols {
		cik, ok := tickerToCik[strings.ToUpper(symbol)]
		if !ok {
			continue
		}
		key := fmt.Sprintf("%010d", cik)
		cikToSymbols[key] = append(cikToSymbols[key], strings.ToUpper(symbol))
	}
	if len(cikToSymbols) == 0 {
		return map[string]int64{}, map[string]string{}
	}

	// sharesOutstanding es una cifra por-entidad (CIK), no por-ticker: se
	// aplica a todos los tickers del CIK (el principal y sus warrants/
	// preferentes del mismo emisor). Descartar el CIK con mas de un ticker
	// dejaba al principal legitimo sin shares (T, SMCI, OPEN, BBD), peor que
	// asignar el SO del emisor a sus instrumentos.
	totalTargets := len(cikToSymbols)

	zipPath, err := c.ensureCachedZip(ctx)
	if err != nil {
		log.Error().Err(err).Str("cacheDir", c.cacheDir).Msg("sec edgar companyfacts.zip unavailable")
		return map[string]int64{}, map[string]string{}
	}
	return parseCompanyFactsZip(zipPath, cikToSymbols, totalTargets)
}
